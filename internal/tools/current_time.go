package tools

import (
	"context"
	"time"

	"mini-agent-runtime/internal/ollama"
)

type CurrentTimeAgentTool struct {
	now func() time.Time
}

// NewCurrentTimeTool 创建 current_time 工具实例，并允许注入时间函数方便测试。
func NewCurrentTimeTool(now func() time.Time) Tool {
	return CurrentTimeAgentTool{now: now}
}

// Name 返回 current_time 工具注册到模型和运行时中的名称。
func (t CurrentTimeAgentTool) Name() string {
	return "current_time"
}

// Description 返回 current_time 工具给模型理解用途的说明。
func (t CurrentTimeAgentTool) Description() string {
	return "获取当前系统时间。当用户询问现在几点、今天日期、当前时间时使用。"
}

// Definition 返回 current_time 工具的 Ollama function calling 定义。
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

// Execute 调用 current_time 工具并返回格式化后的当前时间。
func (t CurrentTimeAgentTool) Execute(context.Context, map[string]any) (string, error) {
	return CurrentTimeTool(t.now), nil
}

// CurrentTimeTool 是一个“获取当前时间”的孤立工具函数。
//
// now 从外部注入，而不是在函数内部直接调用 time.Now。
// 这样真实运行时可以传 time.Now，测试时可以传固定时间，避免测试结果随时间变化。
// CurrentTimeTool 返回当前时间的标准字符串表示，并支持注入 now 函数保证测试稳定。
func CurrentTimeTool(now func() time.Time) string {
	if now == nil {
		now = time.Now
	}
	return now().Format("2006-01-02 15:04:05 MST")
}
