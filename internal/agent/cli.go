package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	apperrors "mini-agent-runtime/internal/errors"
	modelclient "mini-agent-runtime/internal/model"
	"mini-agent-runtime/internal/ollama"
	"mini-agent-runtime/internal/tools"
	tracing "mini-agent-runtime/internal/trace"
)

const maxToolRounds = 4

type Mode string

const (
	ModeChat       Mode = "chat"
	ModePlan       Mode = "plan"
	ModeStrictPlan Mode = "strict-plan"
)

// RunChatLoop 使用默认 trace 配置启动命令行多轮对话流程。
func RunChatLoop(endpoint string, model string, think bool, client *http.Client, initialArgs []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	return RunChatLoopWithTrace(endpoint, model, think, client, initialArgs, stdin, stdout, stderr, tracing.NewTraceHooks(tracing.NewTraceLogger(false, stderr)))
}

// RunChatLoopWithTrace 使用外部传入的 trace hooks 启动命令行多轮对话流程。
func RunChatLoopWithTrace(endpoint string, model string, think bool, client *http.Client, initialArgs []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, traceHooks *tracing.TraceHooks) error {
	return RunChatLoopWithOptions(ChatLoopOptions{
		Endpoint:    endpoint,
		Model:       model,
		Think:       think,
		Client:      client,
		InitialArgs: initialArgs,
		Stdin:       stdin,
		Stdout:      stdout,
		Stderr:      stderr,
		Trace:       traceHooks,
	})
}

type ChatLoopOptions struct {
	Endpoint    string
	Model       string
	Think       bool
	Client      *http.Client
	InitialArgs []string
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	Trace       *tracing.TraceHooks
	Debug       bool
	Mode        Mode
}

// RunChatLoopWithOptions 根据完整配置启动 CLI，对用户输入、模型流式响应和工具调用进行编排。
func RunChatLoopWithOptions(options ChatLoopOptions) error {
	endpoint := options.Endpoint
	model := options.Model
	think := options.Think
	client := options.Client
	stdin := options.Stdin
	stdout := options.Stdout
	stderr := options.Stderr
	traceHooks := options.Trace
	mode := options.Mode
	if mode == "" {
		mode = ModeChat
	}

	if client == nil {
		client = http.DefaultClient
	}
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if traceHooks == nil {
		traceHooks = tracing.NewTraceHooks(tracing.NewTraceLogger(false, stderr))
	}

	reporter := apperrors.NewReporter(options.Debug, stderr)
	toolRegistry := tools.NewDefaultToolRegistry(time.Now)
	modelClient := modelclient.NewClient(modelclient.Options{
		Endpoint: endpoint,
		Model:    model,
		Think:    think,
		HTTP:     client,
		Trace:    traceHooks,
	})
	runtime := NewRuntime(RuntimeOptions{
		ModelClient: modelClient,
		Tools:       toolRegistry,
		Trace:       traceHooks,
		Reporter:    reporter,
		Stdout:      stdout,
	})
	traceHooks.ChatLoopStart(tracing.ChatLoopStartTrace{
		Endpoint: endpoint,
		Model:    model,
		Think:    think,
		Tools:    len(toolRegistry.Definitions()),
	})

	var messages []ollama.Message
	scanner := bufio.NewScanner(stdin)
	pending := strings.TrimSpace(strings.Join(options.InitialArgs, " "))

	for {
		if pending == "" {
			fmt.Fprint(stderr, "You: ")
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					return apperrors.Wrap(apperrors.NodeAgentLoop, apperrors.CodeInvalidUserInput, err, "read message")
				}
				return nil
			}
			pending = strings.TrimSpace(scanner.Text())
		}

		if isExitCommand(pending) {
			traceHooks.ChatLoopExit(tracing.ChatLoopExitTrace{Command: pending})
			return nil
		}
		if pending == "" {
			continue
		}

		messages = append(messages, ollama.Message{Role: "user", Content: pending})
		traceHooks.TurnInput(tracing.TurnInputTrace{Message: pending, HistoryMessages: len(messages)})

		var assistantMessage string
		var err error
		// Planner / Executor 模式
		if isPlannerMode(mode) {
			if mode == ModePlan {
				assistantMessage, err = runtime.RunPlannerExecutorTurn(context.Background(), pending)
			}
			if mode == ModeStrictPlan {
				assistantMessage, err = runtime.RunStrictPlannerExecutorTurn(context.Background(), pending)
			}
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(stdout)
			if assistantMessage != "" {
				messages = append(messages, ollama.Message{Role: "assistant", Content: assistantMessage})
			}
			traceHooks.FinalAnswer(tracing.FinalAnswerTrace{ContentChars: len([]rune(assistantMessage)), HistoryMessages: len(messages)})
			pending = ""
			continue
		}
		if mode != ModeChat {
			return apperrors.New(apperrors.NodeAgentLoop, apperrors.CodeInvalidUserInput, fmt.Sprintf("unknown agent mode: %s", mode))
		}

		// 普通模式
		for toolRound := 0; ; toolRound++ {
			if toolRound >= maxToolRounds {
				return apperrors.New(apperrors.NodeAgentLoop, apperrors.CodeConversationLimit, "too many tool calls in one turn")
			}

			toolContext := context.Background()
			toolDefinitions := toolRegistry.Definitions()
			result, err := modelClient.Chat(toolContext, modelclient.ChatOptions{
				Phase:     "chat",
				ToolRound: toolRound,
				Messages:  messages,
				Tools:     toolDefinitions,
				Stream:    ollama.StreamOptions{Writer: stdout},
			})
			if err != nil {
				return err
			}
			assistantMessage := result.Content
			toolCalls := result.ToolCalls

			if len(toolCalls) == 0 {
				_, _ = fmt.Fprintln(stdout)
				if assistantMessage != "" {
					messages = append(messages, ollama.Message{Role: "assistant", Content: assistantMessage})
				}
				traceHooks.FinalAnswer(tracing.FinalAnswerTrace{ContentChars: len([]rune(assistantMessage)), HistoryMessages: len(messages)})
				break
			}

			messages = append(messages, ollama.Message{
				Role:      "assistant",
				Content:   assistantMessage,
				ToolCalls: toolCalls,
			})
			for _, call := range toolCalls {
				traceHooks.ToolCall(tracing.ToolCallTrace{Name: call.Function.Name, Arguments: call.Function.Arguments})
				result := executeToolCallForModel(toolContext, toolRegistry, call, traceHooks, reporter)
				traceHooks.ToolResult(tracing.ToolResultTrace{Name: call.Function.Name, Result: result})
				messages = append(messages, ollama.Message{
					Role:     "tool",
					Content:  result,
					ToolName: call.Function.Name,
				})
			}
		}

		pending = ""
	}
}

// executeToolCallForModel 执行模型请求的工具调用，并把工具错误格式化成模型可理解的 observation。
func executeToolCallForModel(ctx context.Context, toolRegistry *tools.ToolRegistry, call ollama.ToolCall, traceHooks *tracing.TraceHooks, reporter *apperrors.Reporter) string {
	result, err := toolRegistry.Execute(ctx, call)
	if err != nil {
		err = apperrors.Wrap(apperrors.NodeAgentToolCall, apperrors.CodeToolExecutionFailed, err, "tool call failed")
		traceHooks.ToolError(tracing.ToolErrorTrace{Name: call.Function.Name, Error: err})
		reporter.Debug(err)
		return apperrors.FormatForModel(err)
	}
	return result
}

// isExitCommand 判断用户输入是否是退出 CLI 对话的命令。
func isExitCommand(message string) bool {
	switch strings.ToLower(strings.TrimSpace(message)) {
	case "/exit", "exit", "/quit", "quit":
		return true
	default:
		return false
	}
}

// isPlannerMode 判断是否是 Planner / Strict Planner 模式，这两种模式的交互流程和普通 Chat 模式不同。
func isPlannerMode(mode Mode) bool {
	return mode == ModePlan || mode == ModeStrictPlan
}
