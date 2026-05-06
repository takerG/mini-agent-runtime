package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxToolRounds = 4

func RunChatLoop(endpoint string, model string, think bool, client *http.Client, initialArgs []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	return RunChatLoopWithTrace(endpoint, model, think, client, initialArgs, stdin, stdout, stderr, NewTraceHooks(NewTraceLogger(false, stderr)))
}

func RunChatLoopWithTrace(endpoint string, model string, think bool, client *http.Client, initialArgs []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, trace *TraceHooks) error {
	// client 允许测试注入假的 HTTP 客户端；真实运行时使用 http.DefaultClient。
	// 这类“依赖从外面传进来”的写法，在写 agent 时很常见，因为后续可能要替换模型服务、
	// 记录请求日志、增加重试，或者在测试里模拟模型响应。
	if client == nil {
		client = http.DefaultClient
	}
	toolRegistry := DefaultToolRegistry(time.Now)
	trace.ChatLoopStart(ChatLoopStartTrace{
		Endpoint: endpoint,
		Model:    model,
		Think:    think,
		Tools:    len(toolRegistry.Definitions()),
	})

	// messages 就是多轮对话的记忆。
	// 每轮用户输入会追加一条 role=user；
	// 每轮模型完整回复会追加一条 role=assistant。
	// 下一轮请求会把整个 messages 发给模型，于是模型就能“看见之前聊过什么”。
	var messages []chatMessage

	// scanner 负责从命令行一行一行读取用户输入。
	// 命令行交互里，一行就是一轮 user message。
	scanner := bufio.NewScanner(stdin)

	// 如果用户运行：
	//   go run . "你好"
	// initialArgs 会作为第一轮输入。第一轮结束后，程序仍然继续进入交互循环。
	pending := strings.TrimSpace(strings.Join(initialArgs, " "))

	for {
		if pending == "" {
			// 提示符写到 stderr，而不是 stdout。
			// 这样 stdout 可以只保留模型输出，未来如果用户把输出通过管道交给别的程序，
			// 不会混入 "You:" 这种提示文字。
			fmt.Fprint(stderr, "You: ")
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					return fmt.Errorf("read message: %w", err)
				}
				return nil
			}
			pending = strings.TrimSpace(scanner.Text())
		}

		// 提供几个常见退出命令。EOF 也会退出，例如 Ctrl+Z 后回车（Windows）或 Ctrl+D（Unix）。
		if isExitCommand(pending) {
			trace.ChatLoopExit(ChatLoopExitTrace{Command: pending})
			return nil
		}
		if pending == "" {
			continue
		}

		// 把这一轮用户输入加入历史，再用完整历史构造请求。
		messages = append(messages, chatMessage{Role: "user", Content: pending})
		trace.TurnInput(TurnInputTrace{Message: pending, HistoryMessages: len(messages)})

		for toolRound := 0; ; toolRound++ {
			if toolRound >= maxToolRounds {
				return fmt.Errorf("too many tool calls in one turn")
			}

			// think=true 时，模型服务隐藏 think 流；think=false 时，模型服务显示 think 流。
			// 这个值由 CLI 启动参数传入，而不是在循环里写死。
			req, err := NewChatRequestWithMessagesThinkToolsAndContext(context.Background(), endpoint, model, messages, &think, toolRegistry.Definitions())
			if err != nil {
				return err
			}
			trace.ModelRequest(ModelRequestTrace{ToolRound: toolRound, Messages: len(messages), Tools: len(toolRegistry.Definitions())})

			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("post chat request: %w", err)
			}

			if resp.StatusCode < 200 || resp.StatusCode > 299 {
				resp.Body.Close()
				return fmt.Errorf("chat request failed: %s", resp.Status)
			}

			// StreamChatMessageAndCapture 同时做三件事：
			//   1. 每收到模型的一小段 content，就立刻写到 stdout，保持流式速度
			//   2. 把这一轮 assistant 的完整回复拼起来，便于加入历史
			//   3. 捕获模型返回的 tool_calls，交给 Go 代码执行真实工具
			assistantMessage, toolCalls, streamErr := StreamChatMessageAndCapture(resp.Body, stdout)
			closeErr := resp.Body.Close()
			if streamErr != nil {
				return streamErr
			}
			if closeErr != nil {
				return fmt.Errorf("close chat response: %w", closeErr)
			}
			trace.ModelResponse(ModelResponseTrace{ToolRound: toolRound, ContentChars: len([]rune(assistantMessage)), ToolCalls: len(toolCalls)})

			if len(toolCalls) == 0 {
				// 每轮模型回答结束后补一个换行，让下一轮提示符从新行开始。
				fmt.Fprintln(stdout)
				if assistantMessage != "" {
					messages = append(messages, chatMessage{Role: "assistant", Content: assistantMessage})
				}
				trace.FinalAnswer(FinalAnswerTrace{ContentChars: len([]rune(assistantMessage)), HistoryMessages: len(messages)})
				break
			}

			messages = append(messages, chatMessage{
				Role:      "assistant",
				Content:   assistantMessage,
				ToolCalls: toolCalls,
			})
			for _, call := range toolCalls {
				trace.ToolCall(ToolCallTrace{Name: call.Function.Name, Arguments: call.Function.Arguments})
				result := executeToolCallForModel(toolRegistry, call, trace)
				trace.ToolResult(ToolResultTrace{Name: call.Function.Name, Result: result})
				messages = append(messages, chatMessage{
					Role:     "tool",
					Content:  result,
					ToolName: call.Function.Name,
				})
			}
		}

		// 清空 pending，下一次循环就会从 stdin 读取新的用户输入。
		pending = ""
	}
}

func executeToolCallForModel(toolRegistry *ToolRegistry, call toolCall, trace *TraceHooks) string {
	// 工具调用失败时不要把错误继续向外 return，否则整个 CLI 对话会被中断。
	// 对 agent 来说，更合理的做法是把错误包装成一条 role=tool 的消息交还给模型：
	//   - 如果模型调用了不存在的工具，模型可以根据错误改用已有工具，或者直接向用户解释。
	//   - 如果工具参数不合法、工具内部失败，模型也有机会重新组织参数后再试一次。
	// 这就是 agent 常见的“观察结果 observation 回灌给模型”的思路。
	result, err := toolRegistry.Execute(call)
	if err != nil {
		trace.ToolError(ToolErrorTrace{Name: call.Function.Name, Error: err})
		return fmt.Sprintf("tool error: %v", err)
	}
	return result
}

func isExitCommand(message string) bool {
	switch strings.ToLower(strings.TrimSpace(message)) {
	case "/exit", "exit", "/quit", "quit":
		return true
	default:
		return false
	}
}
