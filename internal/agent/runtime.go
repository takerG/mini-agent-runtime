package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	apperrors "mini-agent-runtime/internal/errors"
	"mini-agent-runtime/internal/executor"
	modelclient "mini-agent-runtime/internal/model"
	"mini-agent-runtime/internal/ollama"
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

// NewRuntime 创建 agent 运行时，并为未显式传入的依赖提供默认实现。
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

// RunPlannerExecutorTurn 执行 Hybrid Planner/Executor 流程，由模型规划后再通过原生 tool calling 完成执行。
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

type strictObservation struct {
	ToolName string
	Result   string
}

// RunStrictPlannerExecutorTurn 执行 Strict Planner/Executor 流程，由 Go 解析计划并直接调用工具。
func (r *Runtime) RunStrictPlannerExecutorTurn(ctx context.Context, userMessage string) (string, error) {
	r.trace.PlannerRequest(tracing.PlannerRequestTrace{Message: userMessage, MessageChars: len([]rune(userMessage))})
	planResult, err := r.modelClient.Chat(ctx, modelclient.ChatOptions{
		Phase: "strict_planner",
		Messages: []ollama.Message{
			{Role: "system", Content: strictPlannerSystemPrompt()},
			{Role: "user", Content: userMessage},
		},
		Stream: ollama.StreamOptions{Writer: io.Discard},
	})
	if err != nil {
		return "", err
	}
	plan, err := planner.ParseExecutablePlan(planResult.Content)
	if err != nil {
		return "", err
	}
	r.trace.PlannerResponse(tracing.PlannerResponseTrace{Goal: plan.Goal, Steps: len(plan.Steps)})

	observations := make([]strictObservation, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		r.trace.ToolCall(tracing.ToolCallTrace{Name: step.ToolName, Arguments: step.Arguments})
		result, err := r.tools.Execute(ctx, ollama.ToolCall{
			Function: ollama.ToolFunctionCall{
				Name:      step.ToolName,
				Arguments: step.Arguments,
			},
		})
		if err != nil {
			err = apperrors.Wrap(apperrors.NodeAgentToolCall, apperrors.CodeToolExecutionFailed, err, "strict planner tool call failed")
			r.trace.ToolError(tracing.ToolErrorTrace{Name: step.ToolName, Error: err})
			r.reporter.Debug(err)
			result = apperrors.FormatForModel(err)
		}
		r.trace.ToolResult(tracing.ToolResultTrace{Name: step.ToolName, Result: result})
		observations = append(observations, strictObservation{ToolName: step.ToolName, Result: result})
	}

	r.printStrictProcess(plan, observations)
	summary, err := r.modelClient.Chat(ctx, modelclient.ChatOptions{
		Phase: "strict_summary",
		Messages: []ollama.Message{
			{Role: "system", Content: strictSummarySystemPrompt(plan, observations)},
			{Role: "user", Content: userMessage},
		},
		Stream: ollama.StreamOptions{Writer: r.stdout},
	})
	if err != nil {
		return "", err
	}
	return summary.Content, nil
}

// printStrictProcess 在 strict-plan 模式下输出计划、工具观测和最终回答标题。
func (r *Runtime) printStrictProcess(plan planner.ExecutablePlan, observations []strictObservation) {
	_, _ = fmt.Fprintln(r.stdout, "[plan]")
	for i, step := range plan.Steps {
		fmt.Fprintf(r.stdout, "%d. tool_call %s %s\n", i+1, step.ToolName, formatArguments(step.Arguments))
	}
	_, _ = fmt.Fprintln(r.stdout)

	_, _ = fmt.Fprintln(r.stdout, "[observation]")
	for i, observation := range observations {
		fmt.Fprintf(r.stdout, "%d. %s -> %s\n", i+1, observation.ToolName, observation.Result)
	}
	_, _ = fmt.Fprintln(r.stdout)

	_, _ = fmt.Fprintln(r.stdout, "Agent:")
}

// strictPlannerSystemPrompt 返回要求模型只生成可执行 JSON 计划的系统提示词。
func strictPlannerSystemPrompt() string {
	return "You are the strict planner in an agent runtime. Return only executable JSON with this shape: {\"goal\":\"short goal\",\"steps\":[{\"type\":\"tool_call\",\"tool_name\":\"calculator\",\"arguments\":{\"a\":23,\"b\":19,\"op\":\"*\"}}]}. Every required external fact or calculation must become a tool_call. Use current_time for current date/time questions. Use calculator for arithmetic. Available tools are current_time and calculator. Do not answer the user directly. Do not call tools yourself."
}

// strictSummarySystemPrompt 根据已执行计划和观测结果生成最终总结阶段的系统提示词。
func strictSummarySystemPrompt(plan planner.ExecutablePlan, observations []strictObservation) string {
	payload := struct {
		Plan         planner.ExecutablePlan `json:"plan"`
		Observations []strictObservation    `json:"observations"`
	}{
		Plan:         plan,
		Observations: observations,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		data = []byte(fmt.Sprintf(`{"goal":%q}`, plan.Goal))
	}
	return "You are the final responder in a strict planner/executor agent runtime. The Go runtime already executed every planned tool call. Use only the observations to answer the user concisely in the user's language. Do not invent missing observations. Do not call tools. Plan and observations:\n" + string(data)
}

// formatArguments 将工具参数格式化成紧凑 JSON，便于在 CLI 过程输出中展示。
func formatArguments(args map[string]any) string {
	if args == nil {
		return "{}"
	}
	data, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(data)
}
