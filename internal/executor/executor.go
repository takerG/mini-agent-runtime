package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"mini-agent-runtime/internal/approval"
	apperrors "mini-agent-runtime/internal/errors"
	"mini-agent-runtime/internal/lifecycle"
	modelclient "mini-agent-runtime/internal/model"
	"mini-agent-runtime/internal/ollama"
	"mini-agent-runtime/internal/planner"
	"mini-agent-runtime/internal/prompts"
	"mini-agent-runtime/internal/tools"
	tracing "mini-agent-runtime/internal/trace"
)

const maxToolRounds = 4

type Options struct {
	ModelClient   *modelclient.Client
	Dependencies  Dependencies
	ShowProcess   bool
	MemoryContext string
}

type Executor struct {
	modelClient   *modelclient.Client
	registry      *tools.ToolRegistry
	toolPolicy    tools.ExecutionPolicy
	trace         *tracing.TraceHooks
	reporter      *apperrors.Reporter
	stdout        io.Writer
	showProcess   bool
	memoryContext string
	recorder      *lifecycle.Recorder
	parentStepID  string
}

// NewExecutor 创建执行器，并补齐输出、trace 和 reporter 的默认依赖。
func NewExecutor(options Options) *Executor {
	dependencies := normalizeDependencies(options.Dependencies)

	return &Executor{
		modelClient:   options.ModelClient,
		registry:      dependencies.Registry,
		toolPolicy:    dependencies.ToolPolicy,
		trace:         dependencies.Trace,
		reporter:      dependencies.Reporter,
		stdout:        dependencies.Stdout,
		showProcess:   options.ShowProcess,
		memoryContext: options.MemoryContext,
		recorder:      dependencies.Recorder,
		parentStepID:  dependencies.ParentStepID,
	}
}

// Execute 根据 planner 生成的计划驱动模型和工具调用，并返回最终面向用户的回答。
func (e *Executor) Execute(ctx context.Context, userMessage string, plan planner.Plan) (string, error) {
	if e.registry == nil {
		return "", missingRegistryError("executor")
	}

	e.trace.ExecutorStart(tracing.ExecutorStartTrace{Steps: len(plan.Steps)})
	for i, step := range plan.Steps {
		e.trace.ExecutorStep(tracing.ExecutorStepTrace{
			Index:    i + 1,
			Task:     step.Task,
			ToolHint: step.ToolHint,
		})
	}

	messages := []ollama.Message{
		{Role: "system", Content: executorSystemPrompt(plan)},
	}
	if e.memoryContext != "" {
		messages = append(messages, ollama.Message{Role: "system", Content: e.memoryContext})
	}
	messages = append(messages, ollama.Message{Role: "user", Content: userMessage})
	processOutput := newExecutorProcessOutput(e, e.showProcess)

	for toolRound := 0; ; toolRound++ {
		if toolRound >= maxToolRounds {
			return "", apperrors.New(apperrors.NodeAgentLoop, apperrors.CodeConversationLimit, "too many executor tool calls")
		}

		processOutput.ResetPrintState()
		streamOptions := ollama.StreamOptions{Writer: e.stdout}
		if e.showProcess {
			streamOptions.ContentStart = processOutput
		}

		step, stepTrace := e.startStep(lifecycle.StepTypeModelRequest, "executor.model", map[string]any{"tool_round": toolRound})
		result, err := e.modelClient.Chat(ctx, modelclient.ChatOptions{
			Phase:        "executor",
			ToolRound:    toolRound,
			Messages:     messages,
			Tools:        e.registry.Definitions(),
			Stream:       streamOptions,
			TraceContext: e.stepTraceContext(step),
		})
		if err != nil {
			e.finishStep(stepTrace, step, err)
			return "", err
		}
		e.addObservation(stepTrace, step, lifecycle.ObservationTypeModelResponse, "executor.model", result.Content, nil)
		e.finishStep(stepTrace, step, nil)
		assistantMessage := result.Content
		toolCalls := result.ToolCalls

		if len(toolCalls) == 0 {
			processOutput.PrintFinalHeaderIfNeeded()
			e.trace.ExecutorFinish(tracing.ExecutorFinishTrace{ContentChars: len([]rune(assistantMessage))})
			return assistantMessage, nil
		}

		processOutput.AppendToolCalls(toolCalls)

		messages = append(messages, ollama.Message{
			Role:      "assistant",
			Content:   assistantMessage,
			ToolCalls: toolCalls,
		})
		for _, call := range toolCalls {
			toolStep, toolTrace := e.startStep(lifecycle.StepTypeToolCall, call.Function.Name, call.Function.Arguments)
			toolTrace.ToolCall(tracing.ToolCallTrace{Name: call.Function.Name, Arguments: call.Function.Arguments})
			result, toolErr := e.executeToolCall(ctx, call, toolTrace, toolStep)
			toolTrace.ToolResult(tracing.ToolResultTrace{Name: call.Function.Name, Result: result})
			observationType := lifecycle.ObservationTypeToolResult
			if toolErr != nil {
				observationType = lifecycle.ObservationTypeToolError
			}
			e.addObservation(toolTrace, toolStep, observationType, call.Function.Name, result, toolErr)
			e.finishStep(toolTrace, toolStep, toolErr)
			processOutput.AppendObservation(call.Function.Name, result)
			messages = append(messages, ollama.Message{
				Role:     "tool",
				Content:  result,
				ToolName: call.Function.Name,
			})
		}
	}
}

type toolObservation struct {
	Name   string
	Result string
}

type executorProcessOutput struct {
	executor     *Executor
	enabled      bool
	printed      bool
	toolCalls    []ollama.ToolCall
	observations []toolObservation
}

// newExecutorProcessOutput 创建 executor 过程展示状态，供流式输出开始前打印 plan 和 observation。
func newExecutorProcessOutput(executor *Executor, enabled bool) *executorProcessOutput {
	return &executorProcessOutput{
		executor: executor,
		enabled:  enabled,
	}
}

// BeforeContent 在当前模型响应的首个内容 chunk 写出前输出 executor 过程信息。
func (o *executorProcessOutput) BeforeContent() error {
	if !o.enabled {
		return nil
	}
	o.executor.printProcessAndFinalHeader(o.toolCalls, o.observations)
	o.printed = true
	return nil
}

// ResetPrintState 重置当前模型请求轮次的过程输出状态。
func (o *executorProcessOutput) ResetPrintState() {
	o.printed = false
}

// AppendToolCalls 记录 executor 已经发起的工具调用，用于过程展示。
func (o *executorProcessOutput) AppendToolCalls(toolCalls []ollama.ToolCall) {
	o.toolCalls = append(o.toolCalls, toolCalls...)
}

// AppendObservation 记录 executor 已经收到的工具观察结果，用于过程展示。
func (o *executorProcessOutput) AppendObservation(name string, result string) {
	o.observations = append(o.observations, toolObservation{Name: name, Result: result})
}

// PrintFinalHeaderIfNeeded 在模型没有输出内容 chunk 时兜底打印最终回答标题。
func (o *executorProcessOutput) PrintFinalHeaderIfNeeded() {
	if !o.enabled || o.printed {
		return
	}
	_ = o.BeforeContent()
}

// printPlan 将 executor 实际发起的工具调用按 plan 区块输出到 CLI。
func (e *Executor) printPlan(toolCalls []ollama.ToolCall) {
	_, _ = fmt.Fprintln(e.stdout, "[plan]")
	for i, call := range toolCalls {
		_, _ = fmt.Fprintf(e.stdout, "%d. tool_call %s %s\n", i+1, call.Function.Name, formatToolArguments(call.Function.Arguments))
	}
	_, _ = fmt.Fprintln(e.stdout)
}

// printObservations 将工具执行结果按 observation 区块输出到 CLI。
func (e *Executor) printObservations(observations []toolObservation) {
	_, _ = fmt.Fprintln(e.stdout, "[observation]")
	for i, observation := range observations {
		_, _ = fmt.Fprintf(e.stdout, "%d. %s -> %s\n", i+1, observation.Name, observation.Result)
	}
	_, _ = fmt.Fprintln(e.stdout)
}

// printProcessAndFinalHeader 输出 plan/observation 过程信息，并打印最终回答标题。
func (e *Executor) printProcessAndFinalHeader(toolCalls []ollama.ToolCall, observations []toolObservation) {
	if len(toolCalls) > 0 {
		e.printPlan(toolCalls)
		e.printObservations(observations)
	}
	_, _ = fmt.Fprintln(e.stdout, "Agent:")
}

// formatToolArguments 将工具调用参数格式化成紧凑 JSON，便于展示执行过程。
func formatToolArguments(args map[string]any) string {
	if args == nil {
		return "{}"
	}
	data, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// executeToolCall 执行单次工具调用，并把失败结果转换成模型可继续处理的文本。
func (e *Executor) executeToolCall(ctx context.Context, call ollama.ToolCall, traceHooks *tracing.TraceHooks, step lifecycle.Step) (string, error) {
	result, err := e.registry.ExecuteWithPolicy(ctx, call, e.approvalPolicyForCurrentStep(traceHooks, step))
	if err != nil {
		err = apperrors.Wrap(apperrors.NodeAgentToolCall, apperrors.CodeToolExecutionFailed, err, "executor tool call failed")
		traceHooks.ToolError(tracing.ToolErrorTrace{Name: call.Function.Name, Error: err})
		e.reporter.Debug(err)
		return apperrors.FormatForModel(err), err
	}
	return result, nil
}

// approvalPolicyForCurrentStep 把 executor 当前 tool step 的上下文注入审批策略。
func (e *Executor) approvalPolicyForCurrentStep(traceHooks *tracing.TraceHooks, step lifecycle.Step) tools.ExecutionPolicy {
	policy := e.toolPolicy
	policy.Approval.Context = approval.RuntimeContext{
		RunID:        e.recorder.RunID(),
		StepID:       step.ID,
		ParentStepID: step.ParentID,
	}
	policy.Approval.Recorder = approval.NewLifecycleRecorder(traceHooks, e.recorder, step)
	return policy
}

// startStep 在 executor 内部创建可观测 step，并返回带 step 上下文的 trace hooks。
func (e *Executor) startStep(stepType lifecycle.StepType, name string, metadata map[string]any) (lifecycle.Step, *tracing.TraceHooks) {
	if e.recorder == nil {
		return lifecycle.Step{}, e.trace
	}
	step := e.recorder.StartStep(e.parentStepID, stepType, name, metadata)
	stepTrace := e.trace.WithContext(e.stepTraceContext(step))
	stepTrace.StepStart(tracing.StepStartTrace{Type: string(step.Type), Name: step.Name})
	return step, stepTrace
}

// finishStep 结束 executor 内部 step，并输出 step_finish trace。
func (e *Executor) finishStep(traceHooks *tracing.TraceHooks, step lifecycle.Step, err error) {
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

// addObservation 记录 executor 内部 step 观察结果，并输出 observation trace。
func (e *Executor) addObservation(traceHooks *tracing.TraceHooks, step lifecycle.Step, observationType lifecycle.ObservationType, name string, content string, err error) {
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

// stepTraceContext 根据 executor 当前 run 和 step 构造 trace 上下文。
func (e *Executor) stepTraceContext(step lifecycle.Step) tracing.TraceContext {
	if e.recorder == nil {
		return tracing.TraceContext{}
	}
	return tracing.TraceContext{
		RunID:        e.recorder.RunID(),
		StepID:       step.ID,
		ParentStepID: step.ParentID,
	}
}

// executorLifecycleStepByID 从 run 快照中查找指定 step。
func executorLifecycleStepByID(run lifecycle.Run, stepID string) lifecycle.Step {
	for _, step := range run.Steps {
		if step.ID == stepID {
			return step
		}
	}
	return lifecycle.Step{}
}

// executorErrorString 把可选错误转换成稳定字符串。
func executorErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// executorSystemPrompt 根据 planner 输出构造 executor 阶段的系统提示词。
func executorSystemPrompt(plan planner.Plan) string {
	planJSON, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		planJSON = []byte(fmt.Sprintf(`{"goal":%q}`, plan.Goal))
	}
	return prompts.ExecutorSystemPrompt(string(planJSON))
}
