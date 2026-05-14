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
	"mini-agent-runtime/internal/lifecycle"
	"mini-agent-runtime/internal/memory"
	modelclient "mini-agent-runtime/internal/model"
	"mini-agent-runtime/internal/tools"
	tracing "mini-agent-runtime/internal/trace"
)

// Mode 表示 CLI 当前使用的 agent 运行模式。
type Mode string

const (
	ModeChat       Mode = "chat"
	ModePlan       Mode = "plan"
	ModeStrictPlan Mode = "strict-plan"
)

// ChatLoopOptions 描述启动 CLI 多轮对话循环所需的完整配置。
type ChatLoopOptions struct {
	Endpoint     string
	Model        string
	Think        bool
	Client       *http.Client
	InitialArgs  []string
	Stdin        io.Reader
	Stdout       io.Writer
	Stderr       io.Writer
	Trace        *tracing.TraceHooks
	Debug        bool
	Mode         Mode
	Memory       *memory.Manager
	MemoryQuery  memory.Query
	Dependencies ChatLoopDependencies
}

// ChatLoopDependencies 保存 CLI loop 可注入的框架依赖，便于测试和后续替换工具、memory、trace 或错误输出实现。
type ChatLoopDependencies struct {
	Tools      *tools.ToolRegistry
	ToolPolicy tools.ExecutionPolicy
	Memory     *memory.Manager
	Lifecycle  *lifecycle.Factory
	Trace      *tracing.TraceHooks
	Reporter   *apperrors.Reporter
}

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

// RunChatLoopWithOptions 根据完整配置启动 CLI，对用户输入、模型流式响应和 agent runner 进行编排。
func RunChatLoopWithOptions(options ChatLoopOptions) error {
	endpoint := options.Endpoint
	model := options.Model
	think := options.Think
	client := options.Client
	stdin := options.Stdin
	stdout := options.Stdout
	stderr := options.Stderr
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
	dependencies := normalizeChatLoopDependencies(options.Dependencies, options.Trace, options.Memory, options.Debug, stderr)
	traceHooks := dependencies.Trace
	reporter := dependencies.Reporter
	toolRegistry := dependencies.Tools
	toolPolicy := dependencies.ToolPolicy
	memoryManager := dependencies.Memory
	lifecycleFactory := dependencies.Lifecycle
	memoryQuery := defaultMemoryQuery(options.MemoryQuery)
	modelClient := modelclient.NewClient(modelclient.Options{
		Endpoint: endpoint,
		Model:    model,
		Think:    think,
		HTTP:     client,
		Trace:    traceHooks,
	})
	runner, err := NewModeRunner(RunnerOptions{
		Mode:        mode,
		ModelClient: modelClient,
		Tools:       toolRegistry,
		ToolPolicy:  toolPolicy,
		Trace:       traceHooks,
		Reporter:    reporter,
		Stdout:      stdout,
		Memory:      memoryManager,
		MemoryQuery: memoryQuery,
		Lifecycle:   lifecycleFactory,
	})
	if err != nil {
		return err
	}
	session := NewSession(SessionOptions{Memory: memoryManager, MemoryQuery: memoryQuery})

	traceHooks.ChatLoopStart(tracing.ChatLoopStartTrace{
		Endpoint: endpoint,
		Model:    model,
		Think:    think,
		Tools:    len(toolRegistry.Definitions()),
	})

	ctx := context.Background()
	scanner := bufio.NewScanner(stdin)
	pending := strings.TrimSpace(strings.Join(options.InitialArgs, " "))

	for {
		if pending == "" {
			_, _ = fmt.Fprint(stderr, "You: ")
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

		if _, err := runner.RunTurn(ctx, session, pending); err != nil {
			return err
		}
		pending = ""
	}
}

// normalizeChatLoopDependencies 合并显式依赖和旧版 options 字段，并为缺失依赖填充默认实现。
func normalizeChatLoopDependencies(dependencies ChatLoopDependencies, traceHooks *tracing.TraceHooks, memoryManager *memory.Manager, debug bool, stderr io.Writer) ChatLoopDependencies {
	if dependencies.Trace == nil {
		dependencies.Trace = traceHooks
	}
	if dependencies.Trace == nil {
		dependencies.Trace = tracing.NewTraceHooks(tracing.NewTraceLogger(false, stderr))
	}
	if dependencies.Reporter == nil {
		dependencies.Reporter = apperrors.NewReporter(debug, stderr)
	}
	if dependencies.Tools == nil {
		dependencies.Tools = tools.NewDefaultToolRegistry(time.Now)
	}
	if dependencies.ToolPolicy.MaxAttempts == 0 {
		dependencies.ToolPolicy = tools.DefaultExecutionPolicy()
	}
	if dependencies.Memory == nil {
		dependencies.Memory = memoryManager
	}
	if dependencies.Memory == nil {
		dependencies.Memory = memory.NewDefaultManager()
	}
	if dependencies.Lifecycle == nil {
		dependencies.Lifecycle = lifecycle.NewFactory(lifecycle.FactoryOptions{})
	}
	return dependencies
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
