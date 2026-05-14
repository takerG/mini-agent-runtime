package planner

import (
	"context"
	"encoding/json"
	"io"

	apperrors "mini-agent-runtime/internal/errors"
	modelclient "mini-agent-runtime/internal/model"
	"mini-agent-runtime/internal/ollama"
	"mini-agent-runtime/internal/prompts"
	tracing "mini-agent-runtime/internal/trace"
)

// Plan 表示 Hybrid Planner 生成的自然语言执行计划。
type Plan struct {
	Goal  string `json:"goal"`
	Steps []Step `json:"steps"`
}

// Step 表示 Hybrid Planner 中的一步任务，以及可选的工具提示。
type Step struct {
	Task     string `json:"task"`
	ToolHint string `json:"tool_hint,omitempty"`
}

// Planner 负责把用户目标拆解成 executor 可以继续执行的计划。
type Planner struct {
	modelClient   *modelclient.Client
	memoryContext string
	toolHints     []string
	traceContext  tracing.TraceContext
}

// Options 描述创建 Planner 时需要注入的依赖和上下文。
type Options struct {
	ModelClient   *modelclient.Client
	MemoryContext string
	ToolHints     []string
	TraceContext  tracing.TraceContext
}

// NewPlanner 创建 planner 组件，用于把用户目标拆解成执行计划。
func NewPlanner(options Options) *Planner {
	return &Planner{
		modelClient:   options.ModelClient,
		memoryContext: options.MemoryContext,
		toolHints:     append([]string(nil), options.ToolHints...),
		traceContext:  options.TraceContext,
	}
}

// Plan 使用默认 context 为用户输入生成自然语言执行计划。
func (p *Planner) Plan(userMessage string) (Plan, error) {
	return p.PlanWithContext(context.Background(), userMessage)
}

// PlanWithContext 调用模型生成 planner JSON，并解析成内部 Plan 结构。
func (p *Planner) PlanWithContext(ctx context.Context, userMessage string) (Plan, error) {
	messages := []ollama.Message{
		{Role: "system", Content: prompts.PlannerSystemPrompt(p.toolHints)},
	}
	if p.memoryContext != "" {
		messages = append(messages, ollama.Message{Role: "system", Content: p.memoryContext})
	}
	messages = append(messages, ollama.Message{Role: "user", Content: userMessage})
	result, err := p.modelClient.Chat(ctx, modelclient.ChatOptions{
		Phase:        "planner",
		ToolRound:    0,
		Messages:     messages,
		Stream:       ollama.StreamOptions{Writer: io.Discard},
		TraceContext: p.traceContext,
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
