package agent

import (
	"context"
	"fmt"
	"io"
	"time"

	apperrors "mini-agent-runtime/internal/errors"
	"mini-agent-runtime/internal/lifecycle"
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
	ToolPolicy  tools.ExecutionPolicy
	Trace       *tracing.TraceHooks
	Reporter    *apperrors.Reporter
	Stdout      io.Writer
	Runtime     *Runtime
	Memory      *memory.Manager
	MemoryQuery memory.Query
	Lifecycle   *lifecycle.Factory
}

// ChatRunner 负责普通 chat 模式下的原生 tool calling 循环。
type ChatRunner struct {
	modelClient *modelclient.Client
	tools       *tools.ToolRegistry
	toolPolicy  tools.ExecutionPolicy
	trace       *tracing.TraceHooks
	reporter    *apperrors.Reporter
	stdout      io.Writer
	lifecycle   *lifecycle.Factory
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
	mode    Mode
	run     func(*Runtime, context.Context, string, *lifecycle.Recorder) (string, error)
}

// NewModeRunner 根据 mode 创建对应 runner，让 CLI loop 不再关心具体模式分发细节。
func NewModeRunner(options RunnerOptions) (ModeRunner, error) {
	deps := normalizeRunnerOptions(options)
	switch deps.Mode {
	case "", ModeChat:
		return &ChatRunner{
			modelClient: deps.ModelClient,
			tools:       deps.Tools,
			toolPolicy:  deps.ToolPolicy,
			trace:       deps.Trace,
			reporter:    deps.Reporter,
			stdout:      deps.Stdout,
			lifecycle:   deps.Lifecycle,
		}, nil
	case ModePlan:
		return &PlannerRunner{
			plannerModeRunner: newPlannerModeRunner(deps, ModePlan, (*Runtime).runPlannerExecutorTurn),
		}, nil
	case ModeStrictPlan:
		return &StrictPlannerRunner{
			plannerModeRunner: newPlannerModeRunner(deps, ModeStrictPlan, (*Runtime).runStrictPlannerExecutorTurn),
		}, nil
	default:
		return nil, apperrors.New(apperrors.NodeAgentLoop, apperrors.CodeInvalidUserInput, fmt.Sprintf("unknown agent mode: %s", deps.Mode))
	}
}

// newPlannerModeRunner 创建 planner 类模式共享 runner，并注入实际 runtime 编排函数。
func newPlannerModeRunner(options RunnerOptions, mode Mode, run func(*Runtime, context.Context, string, *lifecycle.Recorder) (string, error)) *plannerModeRunner {
	return &plannerModeRunner{
		runtime: options.runtime(),
		trace:   options.Trace,
		stdout:  options.Stdout,
		mode:    mode,
		run:     run,
	}
}

// RunTurn 执行普通 chat 模式的一轮用户输入，并在需要时循环处理模型工具调用。
func (r *ChatRunner) RunTurn(ctx context.Context, session *Session, userMessage string) (result TurnResult, err error) {
	recorder := startLifecycleRun(r.trace, r.lifecycle, ModeChat, userMessage)
	defer func() {
		finishLifecycleRun(r.trace, recorder, result.AssistantMessage, err)
	}()

	session.AppendUserMessage(userMessage)
	r.trace.WithContext(runTraceContext(recorder)).TurnInput(tracing.TurnInputTrace{Message: userMessage, HistoryMessages: len(session.History())})

	for toolRound := 0; ; toolRound++ {
		if toolRound >= maxChatToolRounds {
			return TurnResult{}, apperrors.New(apperrors.NodeAgentLoop, apperrors.CodeConversationLimit, "too many tool calls in one turn")
		}

		messages, err := session.MessagesForModel(ctx)
		if err != nil {
			return TurnResult{}, err
		}
		step, stepTrace := startLifecycleStep(r.trace, recorder, "", lifecycle.StepTypeModelRequest, "chat.model", map[string]any{"tool_round": toolRound})
		chatResult, err := r.modelClient.Chat(ctx, modelclient.ChatOptions{
			Phase:        "chat",
			ToolRound:    toolRound,
			Messages:     messages,
			Tools:        r.tools.Definitions(),
			Stream:       ollama.StreamOptions{Writer: r.stdout},
			TraceContext: stepTraceContext(recorder, step),
		})
		if err != nil {
			finishLifecycleStep(stepTrace, recorder, step, err)
			return TurnResult{}, err
		}
		addLifecycleObservation(stepTrace, recorder, step, lifecycle.ObservationTypeModelResponse, "chat.model", chatResult.Content, nil)
		finishLifecycleStep(stepTrace, recorder, step, nil)

		if len(chatResult.ToolCalls) == 0 {
			_, _ = fmt.Fprintln(r.stdout)
			if chatResult.Content != "" {
				session.AppendAssistantMessage(chatResult.Content)
			}
			if err := session.CommitTurn(ctx, userMessage, chatResult.Content); err != nil {
				return TurnResult{}, err
			}
			r.trace.WithContext(runTraceContext(recorder)).FinalAnswer(tracing.FinalAnswerTrace{ContentChars: len([]rune(chatResult.Content)), HistoryMessages: len(session.History())})
			return TurnResult{AssistantMessage: chatResult.Content}, nil
		}

		session.AppendAssistantToolCallMessage(chatResult.Content, chatResult.ToolCalls)
		for _, call := range chatResult.ToolCalls {
			toolStep, toolTrace := startLifecycleStep(r.trace, recorder, "", lifecycle.StepTypeToolCall, call.Function.Name, call.Function.Arguments)
			toolTrace.ToolCall(tracing.ToolCallTrace{Name: call.Function.Name, Arguments: call.Function.Arguments})
			toolResult, toolErr := executeToolCallForModel(ctx, r.tools, call, toolTrace, r.reporter, r.toolPolicy)
			toolTrace.ToolResult(tracing.ToolResultTrace{Name: call.Function.Name, Result: toolResult})
			observationType := lifecycle.ObservationTypeToolResult
			if toolErr != nil {
				observationType = lifecycle.ObservationTypeToolError
			}
			addLifecycleObservation(toolTrace, recorder, toolStep, observationType, call.Function.Name, toolResult, toolErr)
			finishLifecycleStep(toolTrace, recorder, toolStep, toolErr)
			session.AppendToolMessage(call.Function.Name, toolResult)
		}
	}
}

// RunTurn 执行 planner 类模式的一轮用户输入，具体编排函数由构造 runner 时注入。
func (r *plannerModeRunner) RunTurn(ctx context.Context, session *Session, userMessage string) (result TurnResult, err error) {
	recorder := startLifecycleRun(r.trace, r.runtime.lifecycle, r.mode, userMessage)
	defer func() {
		finishLifecycleRun(r.trace, recorder, result.AssistantMessage, err)
	}()

	session.AppendUserMessage(userMessage)
	r.trace.WithContext(runTraceContext(recorder)).TurnInput(tracing.TurnInputTrace{Message: userMessage, HistoryMessages: len(session.History())})

	assistantMessage, err := r.run(r.runtime, ctx, userMessage, recorder)
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
	r.trace.WithContext(runTraceContext(recorder)).FinalAnswer(tracing.FinalAnswerTrace{ContentChars: len([]rune(assistantMessage)), HistoryMessages: len(session.History())})
	return TurnResult{AssistantMessage: assistantMessage}, nil
}

// executeToolCallForModel 执行模型请求的工具调用，并把工具错误格式化成模型可理解的 observation。
func executeToolCallForModel(ctx context.Context, toolRegistry *tools.ToolRegistry, call ollama.ToolCall, traceHooks *tracing.TraceHooks, reporter *apperrors.Reporter, policy tools.ExecutionPolicy) (string, error) {
	result, err := toolRegistry.ExecuteWithPolicy(ctx, call, policy)
	if err != nil {
		err = apperrors.Wrap(apperrors.NodeAgentToolCall, apperrors.CodeToolExecutionFailed, err, "tool call failed")
		traceHooks.ToolError(tracing.ToolErrorTrace{Name: call.Function.Name, Error: err})
		reporter.Debug(err)
		return apperrors.FormatForModel(err), err
	}
	return result, nil
}

// normalizeRunnerOptions 为 runner 工厂补齐默认依赖，避免构造逻辑散落在 CLI loop 中。
func normalizeRunnerOptions(options RunnerOptions) RunnerOptions {
	if options.Tools == nil {
		options.Tools = tools.NewDefaultToolRegistry(time.Now)
	}
	if options.ToolPolicy.MaxAttempts == 0 {
		options.ToolPolicy = tools.DefaultExecutionPolicy()
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
	if options.Lifecycle == nil {
		options.Lifecycle = lifecycle.NewFactory(lifecycle.FactoryOptions{})
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
		ToolPolicy:  o.ToolPolicy,
		Trace:       o.Trace,
		Reporter:    o.Reporter,
		Stdout:      o.Stdout,
		Memory:      o.Memory,
		MemoryQuery: o.MemoryQuery,
		Lifecycle:   o.Lifecycle,
	})
}
