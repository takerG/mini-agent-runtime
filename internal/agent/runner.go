package agent

import (
	"context"
	"fmt"
	"io"
	"time"

	"mini-agent-runtime/internal/approval"
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
	runtime *Runtime
}

// StrictPlannerRunner 负责 Strict Planner/Executor 模式的单轮执行。
type StrictPlannerRunner struct {
	runtime *Runtime
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
			runtime: deps.runtime(),
		}, nil
	case ModeStrictPlan:
		return &StrictPlannerRunner{
			runtime: deps.runtime(),
		}, nil
	default:
		return nil, apperrors.New(apperrors.NodeAgentLoop, apperrors.CodeInvalidUserInput, fmt.Sprintf("unknown agent mode: %s", deps.Mode))
	}
}

// RunTurn 执行普通 chat 模式的一轮用户输入，并在需要时循环处理模型工具调用。
func (r *ChatRunner) RunTurn(ctx context.Context, session *Session, userMessage string) (TurnResult, error) {
	return runAgentTurn(ctx, turnCoordinatorOptions{
		Mode:                 ModeChat,
		Trace:                r.trace,
		Lifecycle:            r.lifecycle,
		Session:              session,
		UserMessage:          userMessage,
		Stdout:               r.stdout,
		PrintTrailingNewline: true,
		Executor:             r,
	})
}

// ExecuteTurn 执行普通 chat 模式的核心单轮逻辑，供统一 turn coordinator 调用。
func (r *ChatRunner) ExecuteTurn(ctx context.Context, options turnExecutionOptions) (string, error) {
	return r.runChatTurn(ctx, options.Session, options.Recorder)
}

// runChatTurn 执行普通 chat 模式内部模型和工具调用循环。
func (r *ChatRunner) runChatTurn(ctx context.Context, session *Session, recorder *lifecycle.Recorder) (string, error) {
	for toolRound := 0; ; toolRound++ {
		if toolRound >= maxChatToolRounds {
			return "", apperrors.New(apperrors.NodeAgentLoop, apperrors.CodeConversationLimit, "too many tool calls in one turn")
		}

		messages, err := session.MessagesForModel(ctx)
		if err != nil {
			return "", err
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
			return "", err
		}
		addLifecycleObservation(stepTrace, recorder, step, lifecycle.ObservationTypeModelResponse, "chat.model", chatResult.Content, nil)
		finishLifecycleStep(stepTrace, recorder, step, nil)

		if len(chatResult.ToolCalls) == 0 {
			return chatResult.Content, nil
		}

		session.AppendAssistantToolCallMessage(chatResult.Content, chatResult.ToolCalls)
		for _, call := range chatResult.ToolCalls {
			toolStep, toolTrace := startLifecycleStep(r.trace, recorder, "", lifecycle.StepTypeToolCall, call.Function.Name, call.Function.Arguments)
			toolTrace.ToolCall(tracing.ToolCallTrace{Name: call.Function.Name, Arguments: call.Function.Arguments})
			toolResult, toolErr := executeToolCallForModel(ctx, r.tools, call, toolTrace, r.reporter, approvalPolicyForStep(r.toolPolicy, toolTrace, recorder, toolStep))
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

// RunTurn 执行 Hybrid Planner/Executor 模式的一轮用户输入。
func (r *PlannerRunner) RunTurn(ctx context.Context, session *Session, userMessage string) (TurnResult, error) {
	return runAgentTurn(ctx, turnCoordinatorOptions{
		Mode:                 ModePlan,
		Trace:                r.runtime.trace,
		Lifecycle:            r.runtime.lifecycle,
		Session:              session,
		UserMessage:          userMessage,
		Stdout:               r.runtime.stdout,
		PrintTrailingNewline: true,
		Executor:             r,
	})
}

// ExecuteTurn 执行 Hybrid Planner/Executor 模式的核心单轮逻辑。
func (r *PlannerRunner) ExecuteTurn(ctx context.Context, options turnExecutionOptions) (string, error) {
	return r.runtime.runPlannerExecutorTurn(ctx, options.UserMessage, options.Recorder)
}

// RunTurn 执行 Strict Planner/Executor 模式的一轮用户输入。
func (r *StrictPlannerRunner) RunTurn(ctx context.Context, session *Session, userMessage string) (TurnResult, error) {
	return runAgentTurn(ctx, turnCoordinatorOptions{
		Mode:                 ModeStrictPlan,
		Trace:                r.runtime.trace,
		Lifecycle:            r.runtime.lifecycle,
		Session:              session,
		UserMessage:          userMessage,
		Stdout:               r.runtime.stdout,
		PrintTrailingNewline: true,
		Executor:             r,
	})
}

// ExecuteTurn 执行 Strict Planner/Executor 模式的核心单轮逻辑。
func (r *StrictPlannerRunner) ExecuteTurn(ctx context.Context, options turnExecutionOptions) (string, error) {
	return r.runtime.runStrictPlannerExecutorTurn(ctx, options.UserMessage, options.Recorder)
}

// approvalPolicyForStep 把当前 tool step 的 run/step 上下文注入工具执行策略。
func approvalPolicyForStep(policy tools.ExecutionPolicy, traceHooks *tracing.TraceHooks, recorder *lifecycle.Recorder, step lifecycle.Step) tools.ExecutionPolicy {
	policy.Approval.Context = approval.RuntimeContext{
		RunID:        recorder.RunID(),
		StepID:       step.ID,
		ParentStepID: step.ParentID,
	}
	policy.Approval.Recorder = approval.NewLifecycleRecorder(traceHooks, recorder, step)
	return policy
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
