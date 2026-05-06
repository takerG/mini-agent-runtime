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
	"mini-agent-runtime/internal/ollama"
	"mini-agent-runtime/internal/tools"
	tracing "mini-agent-runtime/internal/trace"
)

const maxToolRounds = 4

func RunChatLoop(endpoint string, model string, think bool, client *http.Client, initialArgs []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	return RunChatLoopWithTrace(endpoint, model, think, client, initialArgs, stdin, stdout, stderr, tracing.NewTraceHooks(tracing.NewTraceLogger(false, stderr)))
}

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
}

func RunChatLoopWithOptions(options ChatLoopOptions) error {
	endpoint := options.Endpoint
	model := options.Model
	think := options.Think
	client := options.Client
	stdin := options.Stdin
	stdout := options.Stdout
	stderr := options.Stderr
	traceHooks := options.Trace

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

		for toolRound := 0; ; toolRound++ {
			if toolRound >= maxToolRounds {
				return apperrors.New(apperrors.NodeAgentLoop, apperrors.CodeConversationLimit, "too many tool calls in one turn")
			}

			toolContext := context.Background()
			req, err := ollama.NewChatRequestWithMessagesThinkToolsAndContext(toolContext, endpoint, model, messages, &think, toolRegistry.Definitions())
			if err != nil {
				return apperrors.Wrap(apperrors.NodeAgentLoop, apperrors.CodeRequestBuildFailed, err, "build chat request")
			}
			traceHooks.ModelRequest(tracing.ModelRequestTrace{ToolRound: toolRound, Messages: len(messages), Tools: len(toolRegistry.Definitions())})

			resp, err := client.Do(req)
			if err != nil {
				return apperrors.Wrap(apperrors.NodeAgentLoop, apperrors.CodeModelRequestFailed, err, "post chat request")
			}

			if resp.StatusCode < 200 || resp.StatusCode > 299 {
				resp.Body.Close()
				return apperrors.New(apperrors.NodeAgentLoop, apperrors.CodeUpstreamStatusFailed, fmt.Sprintf("chat request failed: %s", resp.Status))
			}

			assistantMessage, toolCalls, streamErr := ollama.StreamChatMessageAndCapture(resp.Body, stdout)
			closeErr := resp.Body.Close()
			if streamErr != nil {
				return apperrors.Wrap(apperrors.NodeAgentLoop, apperrors.CodeModelRequestFailed, streamErr, "stream chat response")
			}
			if closeErr != nil {
				return apperrors.Wrap(apperrors.NodeAgentLoop, apperrors.CodeResponseCloseFailed, closeErr, "close chat response")
			}
			traceHooks.ModelResponse(tracing.ModelResponseTrace{ToolRound: toolRound, ContentChars: len([]rune(assistantMessage)), ToolCalls: len(toolCalls)})

			if len(toolCalls) == 0 {
				fmt.Fprintln(stdout)
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

func isExitCommand(message string) bool {
	switch strings.ToLower(strings.TrimSpace(message)) {
	case "/exit", "exit", "/quit", "quit":
		return true
	default:
		return false
	}
}
