package executor

import (
	"context"
	"fmt"
	"io"

	apperrors "mini-agent-runtime/internal/errors"
	"mini-agent-runtime/internal/lifecycle"
	"mini-agent-runtime/internal/ollama"
	"mini-agent-runtime/internal/planner"
	"mini-agent-runtime/internal/tools"
	tracing "mini-agent-runtime/internal/trace"
)

// StrictExecutorOptions 描述 strict executor 执行可执行计划时需要的依赖。
type StrictExecutorOptions struct {
	Dependencies Dependencies
	ShowProcess  bool
}

// StrictObservation 表示 strict executor 执行单个工具调用后得到的观察结果。
type StrictObservation struct {
	ToolName string `json:"tool_name"`
	Result   string `json:"result"`
}

// StrictExecutor 负责直接执行 strict planner 输出的可执行工具计划。
type StrictExecutor struct {
	registry     *tools.ToolRegistry
	toolPolicy   tools.ExecutionPolicy
	trace        *tracing.TraceHooks
	reporter     *apperrors.Reporter
	stdout       io.Writer
	showProcess  bool
	recorder     *lifecycle.Recorder
	parentStepID string
}

// NewStrictExecutor 创建 strict executor，并为未传入的依赖补齐默认实现。
func NewStrictExecutor(options StrictExecutorOptions) *StrictExecutor {
	dependencies := normalizeDependencies(options.Dependencies)
	return &StrictExecutor{
		registry:     dependencies.Registry,
		toolPolicy:   dependencies.ToolPolicy,
		trace:        dependencies.Trace,
		reporter:     dependencies.Reporter,
		stdout:       dependencies.Stdout,
		showProcess:  options.ShowProcess,
		recorder:     dependencies.Recorder,
		parentStepID: dependencies.ParentStepID,
	}
}

// Execute 按顺序执行可执行计划里的工具调用，并把工具错误转换为可交给模型的 observation。
func (e *StrictExecutor) Execute(ctx context.Context, plan planner.ExecutablePlan) []StrictObservation {
	observations := make([]StrictObservation, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		lifecycleStep, stepTrace := e.startStep(lifecycle.StepTypeToolCall, step.ToolName, step.Arguments)
		stepTrace.ToolCall(tracing.ToolCallTrace{Name: step.ToolName, Arguments: step.Arguments})
		result, err := e.executeToolCall(ctx, step)
		if err != nil {
			err = apperrors.Wrap(apperrors.NodeAgentToolCall, apperrors.CodeToolExecutionFailed, err, "strict executor tool call failed")
			stepTrace.ToolError(tracing.ToolErrorTrace{Name: step.ToolName, Error: err})
			e.reporter.Debug(err)
			result = apperrors.FormatForModel(err)
		}
		stepTrace.ToolResult(tracing.ToolResultTrace{Name: step.ToolName, Result: result})
		observationType := lifecycle.ObservationTypeToolResult
		if err != nil {
			observationType = lifecycle.ObservationTypeToolError
		}
		e.addObservation(stepTrace, lifecycleStep, observationType, step.ToolName, result, err)
		e.finishStep(stepTrace, lifecycleStep, err)
		observations = append(observations, StrictObservation{ToolName: step.ToolName, Result: result})
	}
	if e.showProcess {
		e.printProcess(plan, observations)
	}
	return observations
}

// executeToolCall 执行 strict planner 中的单个工具步骤，并在 registry 缺失时返回配置错误。
func (e *StrictExecutor) executeToolCall(ctx context.Context, step planner.ExecutableStep) (string, error) {
	if e.registry == nil {
		return "", missingRegistryError("strict executor")
	}
	return e.registry.ExecuteWithPolicy(ctx, ollama.ToolCall{
		Function: ollama.ToolFunctionCall{
			Name:      step.ToolName,
			Arguments: step.Arguments,
		},
	}, e.toolPolicy)
}

// startStep 在 strict executor 内部创建可观测 step，并返回带 step 上下文的 trace hooks。
func (e *StrictExecutor) startStep(stepType lifecycle.StepType, name string, metadata map[string]any) (lifecycle.Step, *tracing.TraceHooks) {
	if e.recorder == nil {
		return lifecycle.Step{}, e.trace
	}
	step := e.recorder.StartStep(e.parentStepID, stepType, name, metadata)
	stepTrace := e.trace.WithContext(e.stepTraceContext(step))
	stepTrace.StepStart(tracing.StepStartTrace{Type: string(step.Type), Name: step.Name})
	return step, stepTrace
}

// finishStep 结束 strict executor 内部 step，并输出 step_finish trace。
func (e *StrictExecutor) finishStep(traceHooks *tracing.TraceHooks, step lifecycle.Step, err error) {
	if e.recorder == nil || step.ID == "" {
		return
	}
	e.recorder.FinishStep(step.ID, err)
	latest := executorLifecycleStepByID(e.recorder.Run(), step.ID)
	traceHooks.StepFinish(tracing.StepFinishTrace{
		Status:     string(latest.Status),
		Error:      executorErrorString(err),
		DurationMs: latest.Duration.Milliseconds(),
	})
}

// addObservation 记录 strict executor 内部 step 观察结果，并输出 observation trace。
func (e *StrictExecutor) addObservation(traceHooks *tracing.TraceHooks, step lifecycle.Step, observationType lifecycle.ObservationType, name string, content string, err error) {
	if e.recorder == nil || step.ID == "" {
		return
	}
	e.recorder.AddObservation(step.ID, observationType, name, content, err)
	traceHooks.Observation(tracing.ObservationTrace{
		Type:    string(observationType),
		Name:    name,
		Content: content,
		Error:   executorErrorString(err),
	})
}

// stepTraceContext 根据 strict executor 当前 run 和 step 构造 trace 上下文。
func (e *StrictExecutor) stepTraceContext(step lifecycle.Step) tracing.TraceContext {
	if e.recorder == nil {
		return tracing.TraceContext{}
	}
	return tracing.TraceContext{
		RunID:        e.recorder.RunID(),
		StepID:       step.ID,
		ParentStepID: step.ParentID,
	}
}

// printProcess 输出 strict executor 的计划、观察结果和最终回答标题。
func (e *StrictExecutor) printProcess(plan planner.ExecutablePlan, observations []StrictObservation) {
	_, _ = fmt.Fprintln(e.stdout, "[plan]")
	for i, step := range plan.Steps {
		_, _ = fmt.Fprintf(e.stdout, "%d. tool_call %s %s\n", i+1, step.ToolName, formatToolArguments(step.Arguments))
	}
	_, _ = fmt.Fprintln(e.stdout)

	_, _ = fmt.Fprintln(e.stdout, "[observation]")
	for i, observation := range observations {
		_, _ = fmt.Fprintf(e.stdout, "%d. %s -> %s\n", i+1, observation.ToolName, observation.Result)
	}
	_, _ = fmt.Fprintln(e.stdout)

	_, _ = fmt.Fprintln(e.stdout, "Agent:")
}
