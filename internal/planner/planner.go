package planner

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	apperrors "mini-agent-runtime/internal/errors"
	modelclient "mini-agent-runtime/internal/model"
	"mini-agent-runtime/internal/ollama"
)

type Plan struct {
	Goal  string `json:"goal"`
	Steps []Step `json:"steps"`
}

type Step struct {
	Task     string `json:"task"`
	ToolHint string `json:"tool_hint,omitempty"`
}

type Planner struct {
	modelClient *modelclient.Client
}

type Options struct {
	ModelClient *modelclient.Client
}

func NewPlanner(options Options) *Planner {
	return &Planner{modelClient: options.ModelClient}
}

func (p *Planner) Plan(userMessage string) (Plan, error) {
	return p.PlanWithContext(context.Background(), userMessage)
}

func (p *Planner) PlanWithContext(ctx context.Context, userMessage string) (Plan, error) {
	messages := []ollama.Message{
		{Role: "system", Content: plannerSystemPrompt()},
		{Role: "user", Content: userMessage},
	}
	result, err := p.modelClient.Chat(ctx, modelclient.ChatOptions{
		Phase:     "planner",
		ToolRound: 0,
		Messages:  messages,
		Stream:    ollama.StreamOptions{Writer: io.Discard},
	})
	if err != nil {
		return Plan{}, apperrors.Wrap(apperrors.NodeAgentLoop, apperrors.CodeModelRequestFailed, err, "stream planner response")
	}
	plan, err := ParsePlan(result.Content)
	if err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func ParsePlan(content string) (Plan, error) {
	cleaned := strings.TrimSpace(content)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var plan Plan
	if err := json.Unmarshal([]byte(cleaned), &plan); err != nil {
		return Plan{}, apperrors.Wrap(apperrors.NodeAgentLoop, apperrors.CodeModelRequestFailed, err, "parse planner response")
	}
	if plan.Goal == "" {
		plan.Goal = "answer user request"
	}
	if len(plan.Steps) == 0 {
		plan.Steps = []Step{{Task: plan.Goal}}
	}
	return plan, nil
}

func plannerSystemPrompt() string {
	return strings.Join([]string{
		"You are the planner in a planner/executor agent runtime.",
		"Create a concise execution plan for the executor.",
		"Return only JSON with this shape:",
		`{"goal":"short goal","steps":[{"task":"specific step","tool_hint":"optional tool name"}]}`,
		"Available tool hints are current_time and calculator.",
		"Do not execute tools. Do not answer the user directly.",
	}, "\n")
}
