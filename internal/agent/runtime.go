package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
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
	toolDefinitions := r.tools.Definitions()
	planResult, err := r.modelClient.Chat(ctx, modelclient.ChatOptions{
		Phase: "strict_planner",
		Messages: []ollama.Message{
			{Role: "system", Content: strictPlannerSystemPrompt()},
			{Role: "user", Content: userMessage},
		},
		Tools:  toolDefinitions,
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
	prompt := `
你是一个 agent runtime 中的严格规划器 strict planner。

你的唯一职责是：根据 runtime 提供的用户请求、tools 工具清单和上下文，生成一个可以被程序执行的 JSON 计划。

你不能直接回答用户问题。
你不能调用工具。
你不能解释你的思考过程。
你不能输出 Markdown。
你不能输出 JSON 之外的任何文本。

你必须只返回合法 JSON，格式如下：

{
  "goal": "简短描述本次任务目标",
  "steps": [
    {
      "id": "step1",
      "type": "tool_call",
      "tool_name": "工具名称",
      "arguments": {}
    }
  ]
}

请求中会包含 tools 字段，表示当前 runtime 允许使用的工具。

规划规则：

1. 只能使用 tools 中声明的工具。
2. 绝对不能编造不存在的工具。
3. 每个 tool_call 的 tool_name 必须严格等于 tools 中某个工具的 function.name。
4. 每个 tool_call 的 arguments 必须符合该工具在 tools 中声明的 function.parameters 参数结构。
5. 所有必须依赖外部能力完成的事情，都必须规划成 tool_call。
6. 不要自己计算、查询、推测或编造工具结果。
7. 计划中不能包含工具执行结果，因为工具还没有被执行。
8. 如果某一步依赖前一步结果，使用 "$step_id.result" 的形式引用。
9. steps 必须按照执行顺序排列。
10. 不要生成最终回答步骤，最终回答由 executor 或 responder 在工具执行完成后负责。
11. 如果用户请求可以完全通过可用工具完成，则生成完整步骤。
12. 如果用户请求中只有一部分可以通过可用工具完成，只规划可执行部分，不要编造工具。
13. 如果用户请求完全无法通过 tools 完成，返回：

{
  "goal": "unsupported request",
  "steps": []
}

14. 输出必须可以被 JSON.parse 直接解析。
`
	return strings.TrimSpace(prompt)
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
	prompt := `
你是 strict planner/executor runtime 中的最终回答器 responder。

runtime 已经完成了 strict planner 生成的计划，并执行了所有可执行工具调用。

你的职责是：根据用户原始请求、已执行计划和 observations，生成最终面向用户的自然语言回答。

回答规则：

1. 只能基于 runtime 已执行完成的 plan 和 observations 回答用户。
2. 不要编造 observations 中不存在的结果。
3. 不要调用工具。
4. 不要输出新的计划。
5. 不要输出 Markdown。
6. 如果 observations 中包含工具错误，直接用用户能理解的方式解释限制或失败原因。
7. 使用用户问题所使用的语言回答。
8. 回答应简洁、明确。
`
	return strings.TrimSpace(prompt) + "\n\nruntime_context:\n" + string(data)
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
