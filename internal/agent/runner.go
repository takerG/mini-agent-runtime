package agent

import (
	"context"
	"fmt"
	"io"
	"time"

	apperrors "mini-agent-runtime/internal/errors"
	"mini-agent-runtime/internal/memory"
	modelclient "mini-agent-runtime/internal/model"
	"mini-agent-runtime/internal/ollama"
	"mini-agent-runtime/internal/tools"
	tracing "mini-agent-runtime/internal/trace"
)

const maxChatToolRounds = 4

// TurnResult 表示 runner 完成单轮对话后返回给 CLI loop 的结果。
type TurnResult struct {
	AssistantMessage string
}

// ModeRunner 定义不同 agent 模式需要实现的单轮对话执行接口。
type ModeRunner interface {
	RunTurn(ctx context.Context, session *Session, userMessage string) (TurnResult, error)
}

// RunnerOptions 描述构造模式 runner 时需要注入的共享依赖。
type RunnerOptions struct {
	Mode        Mode
	ModelClient *modelclient.Client
	Tools       *tools.ToolRegistry
	Trace       *tracing.TraceHooks
	Reporter    *apperrors.Reporter
	Stdout      io.Writer
	Runtime     *Runtime
	Memory      *memory.Manager
	MemoryQuery memory.Query
}

// ChatRunner 负责普通 chat 模式下的原生 tool calling 循环。
type ChatRunner struct {
	modelClient *modelclient.Client
	tools       *tools.ToolRegistry
	trace       *tracing.TraceHooks
	reporter    *apperrors.Reporter
	stdout      io.Writer
}

// PlannerRunner 负责 Hybrid Planner/Executor 模式的单轮执行。
type PlannerRunner struct {
	*plannerModeRunner
}

// StrictPlannerRunner 负责 Strict Planner/Executor 模式的单轮执行。
type StrictPlannerRunner struct {
	*plannerModeRunner
}

// plannerModeRunner 封装 planner 类模式共享的会话、memory 和最终 trace 收尾逻辑。
type plannerModeRunner struct {
	runtime *Runtime
	trace   *tracing.TraceHooks
	stdout  io.Writer
	run     func(*Runtime, context.Context, string) (string, error)
}

// NewModeRunner 根据 mode 创建对应 runner，让 CLI loop 不再关心具体模式分发细节。
func NewModeRunner(options RunnerOptions) (ModeRunner, error) {
	deps := normalizeRunnerOptions(options)
	switch deps.Mode {
	case "", ModeChat:
		return &ChatRunner{
			modelClient: deps.ModelClient,
			tools:       deps.Tools,
			trace:       deps.Trace,
			reporter:    deps.Reporter,
			stdout:      deps.Stdout,
		}, nil
	case ModePlan:
		return &PlannerRunner{
			plannerModeRunner: newPlannerModeRunner(deps, (*Runtime).RunPlannerExecutorTurn),
		}, nil
	case ModeStrictPlan:
		return &StrictPlannerRunner{
			plannerModeRunner: newPlannerModeRunner(deps, (*Runtime).RunStrictPlannerExecutorTurn),
		}, nil
	default:
		return nil, apperrors.New(apperrors.NodeAgentLoop, apperrors.CodeInvalidUserInput, fmt.Sprintf("unknown agent mode: %s", deps.Mode))
	}
}

// newPlannerModeRunner 创建 planner 类模式共享 runner，并注入实际 runtime 编排函数。
func newPlannerModeRunner(options RunnerOptions, run func(*Runtime, context.Context, string) (string, error)) *plannerModeRunner {
	return &plannerModeRunner{
		runtime: options.runtime(),
		trace:   options.Trace,
		stdout:  options.Stdout,
		run:     run,
	}
}

// RunTurn 执行普通 chat 模式的一轮用户输入，并在需要时循环处理模型工具调用。
func (r *ChatRunner) RunTurn(ctx context.Context, session *Session, userMessage string) (TurnResult, error) {
	session.AppendUserMessage(userMessage)
	r.trace.TurnInput(tracing.TurnInputTrace{Message: userMessage, HistoryMessages: len(session.History())})

	for toolRound := 0; ; toolRound++ {
		if toolRound >= maxChatToolRounds {
			return TurnResult{}, apperrors.New(apperrors.NodeAgentLoop, apperrors.CodeConversationLimit, "too many tool calls in one turn")
		}

		messages, err := session.MessagesForModel(ctx)
		if err != nil {
			return TurnResult{}, err
		}
		result, err := r.modelClient.Chat(ctx, modelclient.ChatOptions{
			Phase:     "chat",
			ToolRound: toolRound,
			Messages:  messages,
			Tools:     r.tools.Definitions(),
			Stream:    ollama.StreamOptions{Writer: r.stdout},
		})
		if err != nil {
			return TurnResult{}, err
		}

		if len(result.ToolCalls) == 0 {
			_, _ = fmt.Fprintln(r.stdout)
			if result.Content != "" {
				session.AppendAssistantMessage(result.Content)
			}
			if err := session.CommitTurn(ctx, userMessage, result.Content); err != nil {
				return TurnResult{}, err
			}
			r.trace.FinalAnswer(tracing.FinalAnswerTrace{ContentChars: len([]rune(result.Content)), HistoryMessages: len(session.History())})
			return TurnResult{AssistantMessage: result.Content}, nil
		}

		session.AppendAssistantToolCallMessage(result.Content, result.ToolCalls)
		for _, call := range result.ToolCalls {
			r.trace.ToolCall(tracing.ToolCallTrace{Name: call.Function.Name, Arguments: call.Function.Arguments})
			toolResult := executeToolCallForModel(ctx, r.tools, call, r.trace, r.reporter)
			r.trace.ToolResult(tracing.ToolResultTrace{Name: call.Function.Name, Result: toolResult})
			session.AppendToolMessage(call.Function.Name, toolResult)
		}
	}
}

// RunTurn 执行 planner 类模式的一轮用户输入，具体编排函数由构造 runner 时注入。
func (r *plannerModeRunner) RunTurn(ctx context.Context, session *Session, userMessage string) (TurnResult, error) {
	session.AppendUserMessage(userMessage)
	r.trace.TurnInput(tracing.TurnInputTrace{Message: userMessage, HistoryMessages: len(session.History())})

	assistantMessage, err := r.run(r.runtime, ctx, userMessage)
	if err != nil {
		return TurnResult{}, err
	}
	_, _ = fmt.Fprintln(r.stdout)
	if assistantMessage != "" {
		session.AppendAssistantMessage(assistantMessage)
	}
	if err := session.CommitTurn(ctx, userMessage, assistantMessage); err != nil {
		return TurnResult{}, err
	}
	r.trace.FinalAnswer(tracing.FinalAnswerTrace{ContentChars: len([]rune(assistantMessage)), HistoryMessages: len(session.History())})
	return TurnResult{AssistantMessage: assistantMessage}, nil
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

// normalizeRunnerOptions 为 runner 工厂补齐默认依赖，避免构造逻辑散落在 CLI loop 中。
func normalizeRunnerOptions(options RunnerOptions) RunnerOptions {
	if options.Tools == nil {
		options.Tools = tools.NewDefaultToolRegistry(time.Now)
	}
	if options.Trace == nil {
		options.Trace = tracing.NewTraceHooks(nil)
	}
	if options.Reporter == nil {
		options.Reporter = apperrors.NewReporter(false, io.Discard)
	}
	if options.Stdout == nil {
		options.Stdout = io.Discard
	}
	if options.Memory == nil {
		options.Memory = memory.NewDefaultManager()
	}
	options.MemoryQuery = defaultMemoryQuery(options.MemoryQuery)
	return options
}

// runtime 返回 runner 使用的 Runtime；如果调用方未传入，则用当前 runner 依赖即时装配。
func (o RunnerOptions) runtime() *Runtime {
	if o.Runtime != nil {
		return o.Runtime
	}
	return NewRuntime(RuntimeOptions{
		ModelClient: o.ModelClient,
		Tools:       o.Tools,
		Trace:       o.Trace,
		Reporter:    o.Reporter,
		Stdout:      o.Stdout,
		Memory:      o.Memory,
		MemoryQuery: o.MemoryQuery,
	})
}
