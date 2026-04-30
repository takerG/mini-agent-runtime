package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func RunChatLoop(endpoint string, model string, think bool, client *http.Client, initialArgs []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	// client 允许测试注入假的 HTTP 客户端；真实运行时使用 http.DefaultClient。
	// 这类“依赖从外面传进来”的写法，在写 agent 时很常见，因为后续可能要替换模型服务、
	// 记录请求日志、增加重试，或者在测试里模拟模型响应。
	if client == nil {
		client = http.DefaultClient
	}

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
			return nil
		}
		if pending == "" {
			continue
		}

		// 把这一轮用户输入加入历史，再用完整历史构造请求。
		messages = append(messages, chatMessage{Role: "user", Content: pending})
		// think=true 时，模型服务隐藏 think 流；think=false 时，模型服务显示 think 流。
		// 这个值由 CLI 启动参数传入，而不是在循环里写死。
		req, err := NewChatRequestWithMessagesThinkAndContext(context.Background(), endpoint, model, messages, &think)
		if err != nil {
			return err
		}

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("post chat request: %w", err)
		}

		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			resp.Body.Close()
			return fmt.Errorf("chat request failed: %s", resp.Status)
		}

		// StreamChatContentAndCapture 同时做两件事：
		//   1. 每收到模型的一小段 content，就立刻写到 stdout，保持流式速度
		//   2. 把这一轮 assistant 的完整回复拼起来，返回给 RunChatLoop
		// 第二点很关键，因为下一轮请求需要把 assistant 回复也放进 messages 历史。
		assistantMessage, streamErr := StreamChatContentAndCapture(resp.Body, stdout)
		closeErr := resp.Body.Close()
		if streamErr != nil {
			return streamErr
		}
		if closeErr != nil {
			return fmt.Errorf("close chat response: %w", closeErr)
		}

		// 每轮模型回答结束后补一个换行，让下一轮提示符从新行开始。
		fmt.Fprintln(stdout)
		if assistantMessage != "" {
			messages = append(messages, chatMessage{Role: "assistant", Content: assistantMessage})
		}

		// 清空 pending，下一次循环就会从 stdin 读取新的用户输入。
		pending = ""
	}
}

func isExitCommand(message string) bool {
	switch strings.ToLower(strings.TrimSpace(message)) {
	case "/exit", "exit", "/quit", "quit":
		return true
	default:
		return false
	}
}
