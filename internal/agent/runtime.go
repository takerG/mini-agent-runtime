package agent

import (
	"context"
	"io"
	"time"

	apperrors "mini-agent-runtime/internal/errors"
	"mini-agent-runtime/internal/executor"
	modelclient "mini-agent-runtime/internal/model"
	"mini-agent-runtime/internal/planner"
	"mini-agent-runtime/internal/tools"
	tracing "mini-agent-runtime/internal/trace"
)

type RuntimeOptions struct {
	ModelClient *modelclient.Client
	Tools       *tools.ToolRegistry
	Trace       *tracing.TraceHooks
	Reporter    *apperrors.Reporter
	Stdout      io.Writer
}

type Runtime struct {
	modelClient *modelclient.Client
	tools       *tools.ToolRegistry
	trace       *tracing.TraceHooks
	reporter    *apperrors.Reporter
	stdout      io.Writer
}

func NewRuntime(options RuntimeOptions) *Runtime {
	toolRegistry := options.Tools
	if toolRegistry == nil {
		toolRegistry = tools.NewDefaultToolRegistry(time.Now)
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
	return &Runtime{
		modelClient: options.ModelClient,
		tools:       toolRegistry,
		trace:       traceHooks,
		reporter:    reporter,
		stdout:      stdout,
	}
}

func (r *Runtime) RunPlannerExecutorTurn(ctx context.Context, userMessage string) (string, error) {
	r.trace.PlannerRequest(tracing.PlannerRequestTrace{Message: userMessage, MessageChars: len([]rune(userMessage))})
	plan, err := planner.NewPlanner(planner.Options{ModelClient: r.modelClient}).PlanWithContext(ctx, userMessage)
	if err != nil {
		return "", err
	}
	r.trace.PlannerResponse(tracing.PlannerResponseTrace{Goal: plan.Goal, Steps: len(plan.Steps)})

	runtimeExecutor := executor.NewExecutor(executor.Options{
		ModelClient: r.modelClient,
		Registry:    r.tools,
		Trace:       r.trace,
		Reporter:    r.reporter,
		Stdout:      r.stdout,
		ShowProcess: true,
	})
	return runtimeExecutor.Execute(ctx, userMessage, plan)
}
