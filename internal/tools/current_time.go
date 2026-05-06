package tools

import (
	"context"
	"time"

	"mini-agent-runtime/internal/ollama"
)

type CurrentTimeAgentTool struct {
	now func() time.Time
}

func NewCurrentTimeTool(now func() time.Time) Tool {
	return CurrentTimeAgentTool{now: now}
}

func (t CurrentTimeAgentTool) Name() string {
	return "current_time"
}

func (t CurrentTimeAgentTool) Description() string {
	return "获取当前系统时间。当用户询问现在几点、今天日期、当前时间时使用。"
}

func (t CurrentTimeAgentTool) Definition() ollama.ToolDefinition {
	return ollama.ToolDefinition{
		Type: "function",
		Function: ollama.ToolDescription{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
}

func (t CurrentTimeAgentTool) Execute(context.Context, map[string]any) (string, error) {
	return CurrentTimeTool(t.now), nil
}

// CurrentTimeTool 是一个“获取当前时间”的孤立工具函数。
//
// now 从外部注入，而不是在函数内部直接调用 time.Now。
// 这样真实运行时可以传 time.Now，测试时可以传固定时间，避免测试结果随时间变化。
func CurrentTimeTool(now func() time.Time) string {
	if now == nil {
		now = time.Now
	}
	return now().Format("2006-01-02 15:04:05 MST")
}
