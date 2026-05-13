package executor

import (
	"context"
	"fmt"
	"io"
	"time"

	apperrors "mini-agent-runtime/internal/errors"
	"mini-agent-runtime/internal/ollama"
	"mini-agent-runtime/internal/planner"
	"mini-agent-runtime/internal/tools"
	tracing "mini-agent-runtime/internal/trace"
)

// StrictExecutorOptions 描述 strict executor 执行可执行计划时需要的依赖。
type StrictExecutorOptions struct {
	Registry    *tools.ToolRegistry
	Trace       *tracing.TraceHooks
	Reporter    *apperrors.Reporter
	Stdout      io.Writer
	ShowProcess bool
}

// StrictObservation 表示 strict executor 执行单个工具调用后得到的观察结果。
type StrictObservation struct {
	ToolName string `json:"tool_name"`
	Result   string `json:"result"`
}

// StrictExecutor 负责直接执行 strict planner 输出的可执行工具计划。
type StrictExecutor struct {
	registry    *tools.ToolRegistry
	trace       *tracing.TraceHooks
	reporter    *apperrors.Reporter
	stdout      io.Writer
	showProcess bool
}

// NewStrictExecutor 创建 strict executor，并为未传入的依赖补齐默认实现。
func NewStrictExecutor(options StrictExecutorOptions) *StrictExecutor {
	registry := options.Registry
	if registry == nil {
		registry = tools.NewDefaultToolRegistry(time.Now)
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
	return &StrictExecutor{
		registry:    registry,
		trace:       traceHooks,
		reporter:    reporter,
		stdout:      stdout,
		showProcess: options.ShowProcess,
	}
}

// Execute 按顺序执行可执行计划里的工具调用，并把工具错误转换为可交给模型的 observation。
func (e *StrictExecutor) Execute(ctx context.Context, plan planner.ExecutablePlan) []StrictObservation {
	observations := make([]StrictObservation, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		e.trace.ToolCall(tracing.ToolCallTrace{Name: step.ToolName, Arguments: step.Arguments})
		result, err := e.registry.Execute(ctx, ollama.ToolCall{
			Function: ollama.ToolFunctionCall{
				Name:      step.ToolName,
				Arguments: step.Arguments,
			},
		})
		if err != nil {
			err = apperrors.Wrap(apperrors.NodeAgentToolCall, apperrors.CodeToolExecutionFailed, err, "strict executor tool call failed")
			e.trace.ToolError(tracing.ToolErrorTrace{Name: step.ToolName, Error: err})
			e.reporter.Debug(err)
			result = apperrors.FormatForModel(err)
		}
		e.trace.ToolResult(tracing.ToolResultTrace{Name: step.ToolName, Result: result})
		observations = append(observations, StrictObservation{ToolName: step.ToolName, Result: result})
	}
	if e.showProcess {
		e.printProcess(plan, observations)
	}
	return observations
}

// printProcess 输出 strict executor 的计划、观察结果和最终回答标题。
func (e *StrictExecutor) printProcess(plan planner.ExecutablePlan, observations []StrictObservation) {
	_, _ = fmt.Fprintln(e.stdout, "[plan]")
	for i, step := range plan.Steps {
		fmt.Fprintf(e.stdout, "%d. tool_call %s %s\n", i+1, step.ToolName, formatToolArguments(step.Arguments))
	}
	_, _ = fmt.Fprintln(e.stdout)

	_, _ = fmt.Fprintln(e.stdout, "[observation]")
	for i, observation := range observations {
		fmt.Fprintf(e.stdout, "%d. %s -> %s\n", i+1, observation.ToolName, observation.Result)
	}
	_, _ = fmt.Fprintln(e.stdout)

	_, _ = fmt.Fprintln(e.stdout, "Agent:")
}
