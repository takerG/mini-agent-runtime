package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	apperrors "mini-agent-runtime/internal/errors"
	"mini-agent-runtime/internal/executor"
	"mini-agent-runtime/internal/lifecycle"
	"mini-agent-runtime/internal/memory"
	modelclient "mini-agent-runtime/internal/model"
	"mini-agent-runtime/internal/ollama"
	"mini-agent-runtime/internal/planner"
	"mini-agent-runtime/internal/prompts"
	"mini-agent-runtime/internal/tools"
	tracing "mini-agent-runtime/internal/trace"
)

// RuntimeOptions 描述创建 agent runtime 时需要注入的共享依赖。
type RuntimeOptions struct {
	ModelClient *modelclient.Client
	Tools       *tools.ToolRegistry
	ToolPolicy  tools.ExecutionPolicy
	Trace       *tracing.TraceHooks
	Reporter    *apperrors.Reporter
	Stdout      io.Writer
	Memory      *memory.Manager
	MemoryQuery memory.Query
	Lifecycle   *lifecycle.Factory
}

// Runtime 负责承载跨模式复用的 agent 编排能力，尤其是 planner/executor 相关流程。
type Runtime struct {
	modelClient *modelclient.Client
	tools       *tools.ToolRegistry
	toolPolicy  tools.ExecutionPolicy
	trace       *tracing.TraceHooks
	reporter    *apperrors.Reporter
	stdout      io.Writer
	memory      *memory.Manager
	memoryQuery memory.Query
	lifecycle   *lifecycle.Factory
}

// NewRuntime 创建 agent 运行时，并为未显式传入的依赖提供默认实现。
func NewRuntime(options RuntimeOptions) *Runtime {
	toolRegistry := options.Tools
	if toolRegistry == nil {
		toolRegistry = tools.NewDefaultToolRegistry(time.Now)
	}
	toolPolicy := options.ToolPolicy
	if toolPolicy.MaxAttempts == 0 {
		toolPolicy = tools.DefaultExecutionPolicy()
	}
	traceHooks := options.Trace
	if traceHooks == nil {
		traceHooks = tracing.NewTraceHooks(nil)
	}
	reporter := options.Reporter
	if reporter == nil {
		reporter = apperrors.NewReporter(false, io.Discard)
	}
	stdout := options.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	memoryManager := options.Memory
	if memoryManager == nil {
		memoryManager = memory.NewDefaultManager()
	}
	lifecycleFactory := options.Lifecycle
	if lifecycleFactory == nil {
		lifecycleFactory = lifecycle.NewFactory(lifecycle.FactoryOptions{})
	}
	memoryQuery := defaultMemoryQuery(options.MemoryQuery)
	return &Runtime{
		modelClient: options.ModelClient,
		tools:       toolRegistry,
		toolPolicy:  toolPolicy,
		trace:       traceHooks,
		reporter:    reporter,
		stdout:      stdout,
		memory:      memoryManager,
		memoryQuery: memoryQuery,
		lifecycle:   lifecycleFactory,
	}
}

// RunPlannerExecutorTurn 执行 Hybrid Planner/Executor 流程，由模型规划后再通过原生 tool calling 完成执行。
func (r *Runtime) RunPlannerExecutorTurn(ctx context.Context, userMessage string) (string, error) {
	recorder := startLifecycleRun(r.trace, r.lifecycle, ModePlan, userMessage)
	var answer string
	var err error
	defer func() {
		finishLifecycleRun(r.trace, recorder, answer, err)
	}()
	answer, err = r.runPlannerExecutorTurn(ctx, userMessage, recorder)
	return answer, err
}

// runPlannerExecutorTurn 执行 Hybrid Planner/Executor 内部流程，并复用调用方传入的生命周期 recorder。
func (r *Runtime) runPlannerExecutorTurn(ctx context.Context, userMessage string, recorder *lifecycle.Recorder) (string, error) {
	plannerStep, plannerTrace := startLifecycleStep(r.trace, recorder, "", lifecycle.StepTypePlanner, "hybrid.planner", nil)
	plannerTrace.PlannerRequest(tracing.PlannerRequestTrace{Message: userMessage, MessageChars: len([]rune(userMessage))})
	memoryContext, err := r.memorySystemMessage(ctx)
	if err != nil {
		finishLifecycleStep(plannerTrace, recorder, plannerStep, err)
		return "", err
	}
	plan, err := planner.NewPlanner(planner.Options{
		ModelClient:   r.modelClient,
		MemoryContext: memoryContext,
		ToolHints:     toolNamesFromDefinitions(r.tools.Definitions()),
		TraceContext:  stepTraceContext(recorder, plannerStep),
	}).PlanWithContext(ctx, userMessage)
	if err != nil {
		finishLifecycleStep(plannerTrace, recorder, plannerStep, err)
		return "", err
	}
	addLifecycleObservation(plannerTrace, recorder, plannerStep, lifecycle.ObservationTypeModelResponse, "hybrid.planner", plan.Goal, nil)
	plannerTrace.PlannerResponse(tracing.PlannerResponseTrace{Goal: plan.Goal, Steps: len(plan.Steps)})
	finishLifecycleStep(plannerTrace, recorder, plannerStep, nil)

	executorStep, executorTrace := startLifecycleStep(r.trace, recorder, "", lifecycle.StepTypeExecutor, "hybrid.executor", map[string]any{"steps": len(plan.Steps)})
	runtimeExecutor := executor.NewExecutor(executor.Options{
		ModelClient: r.modelClient,
		Dependencies: executor.Dependencies{
			Registry:     r.tools,
			ToolPolicy:   r.toolPolicy,
			Trace:        r.trace,
			Reporter:     r.reporter,
			Stdout:       r.stdout,
			Recorder:     recorder,
			ParentStepID: executorStep.ID,
		},
		ShowProcess:   true,
		MemoryContext: memoryContext,
	})
	answer, err := runtimeExecutor.Execute(ctx, userMessage, plan)
	if err != nil {
		finishLifecycleStep(executorTrace, recorder, executorStep, err)
		return "", err
	}
	addLifecycleObservation(executorTrace, recorder, executorStep, lifecycle.ObservationTypeFinalAnswer, "hybrid.executor", answer, nil)
	finishLifecycleStep(executorTrace, recorder, executorStep, nil)
	return answer, nil
}

// RunStrictPlannerExecutorTurn 执行 Strict Planner/Executor 流程，由 Go 解析计划并直接调用工具。
func (r *Runtime) RunStrictPlannerExecutorTurn(ctx context.Context, userMessage string) (string, error) {
	recorder := startLifecycleRun(r.trace, r.lifecycle, ModeStrictPlan, userMessage)
	var answer string
	var err error
	defer func() {
		finishLifecycleRun(r.trace, recorder, answer, err)
	}()
	answer, err = r.runStrictPlannerExecutorTurn(ctx, userMessage, recorder)
	return answer, err
}

// runStrictPlannerExecutorTurn 执行 Strict Planner/Executor 内部流程，并复用调用方传入的生命周期 recorder。
func (r *Runtime) runStrictPlannerExecutorTurn(ctx context.Context, userMessage string, recorder *lifecycle.Recorder) (string, error) {
	plannerStep, plannerTrace := startLifecycleStep(r.trace, recorder, "", lifecycle.StepTypePlanner, "strict.planner", nil)
	plannerTrace.PlannerRequest(tracing.PlannerRequestTrace{Message: userMessage, MessageChars: len([]rune(userMessage))})
	memoryContext, err := r.memorySystemMessage(ctx)
	if err != nil {
		finishLifecycleStep(plannerTrace, recorder, plannerStep, err)
		return "", err
	}
	toolDefinitions := r.tools.Definitions()
	plannerMessages := []ollama.Message{
		{Role: "system", Content: prompts.StrictPlannerSystemPrompt()},
	}
	if memoryContext != "" {
		plannerMessages = append(plannerMessages, ollama.Message{Role: "system", Content: memoryContext})
	}
	plannerMessages = append(plannerMessages, ollama.Message{Role: "user", Content: userMessage})
	planResult, err := r.modelClient.Chat(ctx, modelclient.ChatOptions{
		Phase:        "strict_planner",
		Messages:     plannerMessages,
		Tools:        toolDefinitions,
		Stream:       ollama.StreamOptions{Writer: io.Discard},
		TraceContext: stepTraceContext(recorder, plannerStep),
	})
	if err != nil {
		finishLifecycleStep(plannerTrace, recorder, plannerStep, err)
		return "", err
	}
	plan, err := planner.ParseExecutablePlan(planResult.Content)
	if err != nil {
		finishLifecycleStep(plannerTrace, recorder, plannerStep, err)
		return "", err
	}
	addLifecycleObservation(plannerTrace, recorder, plannerStep, lifecycle.ObservationTypeModelResponse, "strict.planner", plan.Goal, nil)
	plannerTrace.PlannerResponse(tracing.PlannerResponseTrace{Goal: plan.Goal, Steps: len(plan.Steps)})
	finishLifecycleStep(plannerTrace, recorder, plannerStep, nil)

	executorStep, executorTrace := startLifecycleStep(r.trace, recorder, "", lifecycle.StepTypeExecutor, "strict.executor", map[string]any{"steps": len(plan.Steps)})
	observations := executor.NewStrictExecutor(executor.StrictExecutorOptions{
		Dependencies: executor.Dependencies{
			Registry:     r.tools,
			ToolPolicy:   r.toolPolicy,
			Trace:        r.trace,
			Reporter:     r.reporter,
			Stdout:       r.stdout,
			Recorder:     recorder,
			ParentStepID: executorStep.ID,
		},
		ShowProcess: true,
	}).Execute(ctx, plan)
	addLifecycleObservation(executorTrace, recorder, executorStep, lifecycle.ObservationTypeToolResult, "strict.executor", fmt.Sprintf("%d observations", len(observations)), nil)
	finishLifecycleStep(executorTrace, recorder, executorStep, nil)

	summaryStep, summaryTrace := startLifecycleStep(r.trace, recorder, "", lifecycle.StepTypeSummary, "strict.summary", nil)
	summaryMessages := []ollama.Message{
		{Role: "system", Content: prompts.StrictSummarySystemPrompt(strictRuntimeContextJSON(plan, observations))},
	}
	if memoryContext != "" {
		summaryMessages = append(summaryMessages, ollama.Message{Role: "system", Content: memoryContext})
	}
	summaryMessages = append(summaryMessages, ollama.Message{Role: "user", Content: userMessage})
	summary, err := r.modelClient.Chat(ctx, modelclient.ChatOptions{
		Phase:        "strict_summary",
		Messages:     summaryMessages,
		Stream:       ollama.StreamOptions{Writer: r.stdout},
		TraceContext: stepTraceContext(recorder, summaryStep),
	})
	if err != nil {
		finishLifecycleStep(summaryTrace, recorder, summaryStep, err)
		return "", err
	}
	addLifecycleObservation(summaryTrace, recorder, summaryStep, lifecycle.ObservationTypeFinalAnswer, "strict.summary", summary.Content, nil)
	finishLifecycleStep(summaryTrace, recorder, summaryStep, nil)
	return summary.Content, nil
}

// memorySystemMessage 读取 runtime 当前 memory 上下文并格式化成 system message 内容。
func (r *Runtime) memorySystemMessage(ctx context.Context) (string, error) {
	if r == nil || r.memory == nil {
		return "", nil
	}
	memoryContext, err := r.memory.Context(ctx, r.memoryQuery)
	if err != nil {
		return "", err
	}
	return memoryContext.SystemMessage(), nil
}

// strictRuntimeContextJSON 将 strict-plan 已执行计划和观察结果序列化成 responder 可读取的 JSON 上下文。
func strictRuntimeContextJSON(plan planner.ExecutablePlan, observations []executor.StrictObservation) string {
	payload := struct {
		Plan         planner.ExecutablePlan       `json:"plan"`
		Observations []executor.StrictObservation `json:"observations"`
	}{
		Plan:         plan,
		Observations: observations,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"goal":%q}`, plan.Goal)
	}
	return string(data)
}

// toolNamesFromDefinitions 从工具定义中提取稳定排序的工具名，用于 planner prompt 的动态提示。
func toolNamesFromDefinitions(definitions []ollama.ToolDefinition) []string {
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		if definition.Function.Name != "" {
			names = append(names, definition.Function.Name)
		}
	}
	sort.Strings(names)
	return names
}
