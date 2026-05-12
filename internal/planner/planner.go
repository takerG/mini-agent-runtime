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

// NewPlanner 创建 planner 组件，用于把用户目标拆解成执行计划。
func NewPlanner(options Options) *Planner {
	return &Planner{modelClient: options.ModelClient}
}

// Plan 使用默认 context 为用户输入生成自然语言执行计划。
func (p *Planner) Plan(userMessage string) (Plan, error) {
	return p.PlanWithContext(context.Background(), userMessage)
}

// PlanWithContext 调用模型生成 planner JSON，并解析成内部 Plan 结构。
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

// ParsePlan 将模型返回的 planner JSON 文本解析成 Plan，并补齐安全默认值。
func ParsePlan(content string) (Plan, error) {
	cleaned := cleanJSONContent(content)

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

// plannerSystemPrompt 返回自然语言 planner 阶段的系统提示词。
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
