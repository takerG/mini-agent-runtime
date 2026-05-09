package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	apperrors "mini-agent-runtime/internal/errors"
	modelclient "mini-agent-runtime/internal/model"
	"mini-agent-runtime/internal/ollama"
	"mini-agent-runtime/internal/planner"
	"mini-agent-runtime/internal/tools"
	tracing "mini-agent-runtime/internal/trace"
)

const maxToolRounds = 4

type Options struct {
	ModelClient *modelclient.Client
	Registry    *tools.ToolRegistry
	Trace       *tracing.TraceHooks
	Reporter    *apperrors.Reporter
	Stdout      io.Writer
	ShowProcess bool
}

type Executor struct {
	modelClient *modelclient.Client
	registry    *tools.ToolRegistry
	trace       *tracing.TraceHooks
	reporter    *apperrors.Reporter
	stdout      io.Writer
	showProcess bool
}

func NewExecutor(options Options) *Executor {
	stdout := options.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	traceHooks := options.Trace
	if traceHooks == nil {
		traceHooks = tracing.NewTraceHooks(nil)
	}
	reporter := options.Reporter
	if reporter == nil {
		reporter = apperrors.NewReporter(false, io.Discard)
	}

	return &Executor{
		modelClient: options.ModelClient,
		registry:    options.Registry,
		trace:       traceHooks,
		reporter:    reporter,
		stdout:      stdout,
		showProcess: options.ShowProcess,
	}
}

func (e *Executor) Execute(ctx context.Context, userMessage string, plan planner.Plan) (string, error) {
	if e.registry == nil {
		e.registry = tools.NewDefaultToolRegistry(nil)
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
		{Role: "user", Content: userMessage},
	}
	allToolCalls := []ollama.ToolCall{}
	allObservations := []toolObservation{}

	for toolRound := 0; ; toolRound++ {
		if toolRound >= maxToolRounds {
			return "", apperrors.New(apperrors.NodeAgentLoop, apperrors.CodeConversationLimit, "too many executor tool calls")
		}

		processPrinted := false
		streamOptions := ollama.StreamOptions{Writer: e.stdout}
		if e.showProcess {
			streamOptions.BeforeContent = func() error {
				e.printProcessAndFinalHeader(allToolCalls, allObservations)
				processPrinted = true
				return nil
			}
		}

		result, err := e.modelClient.Chat(ctx, modelclient.ChatOptions{
			Phase:     "executor",
			ToolRound: toolRound,
			Messages:  messages,
			Tools:     e.registry.Definitions(),
			Stream:    streamOptions,
		})
		if err != nil {
			return "", err
		}
		assistantMessage := result.Content
		toolCalls := result.ToolCalls

		if len(toolCalls) == 0 {
			if e.showProcess && !processPrinted {
				e.printProcessAndFinalHeader(allToolCalls, allObservations)
			}
			e.trace.ExecutorFinish(tracing.ExecutorFinishTrace{ContentChars: len([]rune(assistantMessage))})
			return assistantMessage, nil
		}

		allToolCalls = append(allToolCalls, toolCalls...)

		messages = append(messages, ollama.Message{
			Role:      "assistant",
			Content:   assistantMessage,
			ToolCalls: toolCalls,
		})
		for _, call := range toolCalls {
			e.trace.ToolCall(tracing.ToolCallTrace{Name: call.Function.Name, Arguments: call.Function.Arguments})
			result := e.executeToolCall(ctx, call)
			e.trace.ToolResult(tracing.ToolResultTrace{Name: call.Function.Name, Result: result})
			observation := toolObservation{Name: call.Function.Name, Result: result}
			allObservations = append(allObservations, observation)
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

func (e *Executor) printPlan(toolCalls []ollama.ToolCall) {
	fmt.Fprintln(e.stdout, "[plan]")
	for i, call := range toolCalls {
		fmt.Fprintf(e.stdout, "%d. tool_call %s %s\n", i+1, call.Function.Name, formatToolArguments(call.Function.Arguments))
	}
	fmt.Fprintln(e.stdout)
}

func (e *Executor) printObservations(observations []toolObservation) {
	fmt.Fprintln(e.stdout, "[observation]")
	for i, observation := range observations {
		fmt.Fprintf(e.stdout, "%d. %s -> %s\n", i+1, observation.Name, observation.Result)
	}
	fmt.Fprintln(e.stdout)
}

func (e *Executor) printProcessAndFinalHeader(toolCalls []ollama.ToolCall, observations []toolObservation) {
	if len(toolCalls) > 0 {
		e.printPlan(toolCalls)
		e.printObservations(observations)
	}
	fmt.Fprintln(e.stdout, "Agent:")
}

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

func (e *Executor) executeToolCall(ctx context.Context, call ollama.ToolCall) string {
	result, err := e.registry.Execute(ctx, call)
	if err != nil {
		err = apperrors.Wrap(apperrors.NodeAgentToolCall, apperrors.CodeToolExecutionFailed, err, "executor tool call failed")
		e.trace.ToolError(tracing.ToolErrorTrace{Name: call.Function.Name, Error: err})
		e.reporter.Debug(err)
		return apperrors.FormatForModel(err)
	}
	return result
}

func executorSystemPrompt(plan planner.Plan) string {
	planJSON, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		planJSON = []byte(fmt.Sprintf(`{"goal":%q}`, plan.Goal))
	}
	return strings.Join([]string{
		"You are the executor in a planner/executor agent runtime.",
		"Follow the plan, call tools when useful, and produce the final user-facing answer.",
		"After tool results are available, synthesize them into a concise natural-language answer in the user's language.",
		"Do not answer with only raw observation values unless the user explicitly asks for raw data.",
		"The CLI renders [plan] and [observation] sections, so do not include those labels yourself.",
		"Execution plan:",
		string(planJSON),
	}, "\n")
}
