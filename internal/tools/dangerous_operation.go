package tools

import (
	"context"
	"fmt"
	"strings"

	"mini-agent-runtime/internal/approval"
	"mini-agent-runtime/internal/ollama"
)

type DangerousOperationAgentTool struct{}

// NewDangerousOperationTool 创建用于验收 Human-in-the-loop 的模拟高危工具实例。
func NewDangerousOperationTool() Tool {
	return DangerousOperationAgentTool{}
}

// Name 返回 dangerous_operation 工具注册到模型和运行时中的名称。
func (DangerousOperationAgentTool) Name() string {
	return "dangerous_operation"
}

// Description 返回 dangerous_operation 工具给模型理解用途的说明。
func (DangerousOperationAgentTool) Description() string {
	return "模拟一个需要人工确认的高危操作。只有用户明确确认后才允许执行，执行后仅返回【高危操作】标记，不产生真实副作用。"
}

// RiskProfile 声明 dangerous_operation 是高风险工具，执行前必须经过人工确认。
func (DangerousOperationAgentTool) RiskProfile() approval.RiskProfile {
	return approval.RiskProfile{
		Level:       approval.RiskLevelHigh,
		Category:    "demo",
		Description: "模拟一个不会产生真实副作用的高危操作。",
		Reasons:     []string{"高危操作需要人工确认"},
	}
}

// Definition 返回 dangerous_operation 工具的 Ollama function calling 定义。
func (t DangerousOperationAgentTool) Definition() ollama.ToolDefinition {
	return ollama.ToolDefinition{
		Type: "function",
		Function: ollama.ToolDescription{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"description": "需要模拟执行的高危动作描述，例如删除文件、发起生产变更或清理数据。",
					},
				},
				"required": []string{"action"},
			},
		},
	}
}

// Execute 返回高危操作标记，用于演示确认后才执行的工具结果。
func (DangerousOperationAgentTool) Execute(_ context.Context, args map[string]any) (string, error) {
	action := dangerousActionDescription(args)
	return fmt.Sprintf("【高危操作】模拟执行完成：%s", action), nil
}

// dangerousActionDescription 从参数中读取高危动作描述，缺失时返回兜底描述。
func dangerousActionDescription(args map[string]any) string {
	value, ok := args["action"]
	if !ok {
		return "未指定动作"
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "未指定动作"
	}
	return strings.TrimSpace(text)
}
