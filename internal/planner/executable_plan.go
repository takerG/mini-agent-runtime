package planner

import (
	"encoding/json"
	"fmt"
	"strings"

	apperrors "mini-agent-runtime/internal/errors"
)

const ExecutableStepToolCall = "tool_call"

type ExecutablePlan struct {
	Goal  string           `json:"goal"`
	Steps []ExecutableStep `json:"steps"`
}

type ExecutableStep struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	ToolName  string         `json:"tool_name"`
	Arguments map[string]any `json:"arguments"`
}

// ParseExecutablePlan 将 strict planner 返回的可执行 JSON 解析成工具调用计划。
func ParseExecutablePlan(content string) (ExecutablePlan, error) {
	cleaned := cleanJSONContent(content)

	var plan ExecutablePlan
	if err := json.Unmarshal([]byte(cleaned), &plan); err != nil {
		return ExecutablePlan{}, apperrors.Wrap(apperrors.NodeAgentLoop, apperrors.CodeModelRequestFailed, err, "parse executable planner response")
	}
	if plan.Goal == "" {
		plan.Goal = "answer user request"
	}
	if len(plan.Steps) == 0 {
		return ExecutablePlan{}, apperrors.New(apperrors.NodeAgentLoop, apperrors.CodeModelRequestFailed, "executable plan has no steps")
	}
	for i := range plan.Steps {
		if plan.Steps[i].Type == "" {
			plan.Steps[i].Type = ExecutableStepToolCall
		}
		if plan.Steps[i].Type != ExecutableStepToolCall {
			return ExecutablePlan{}, apperrors.New(apperrors.NodeAgentLoop, apperrors.CodeModelRequestFailed, fmt.Sprintf("unsupported executable step type: %s", plan.Steps[i].Type))
		}
		if plan.Steps[i].ToolName == "" {
			return ExecutablePlan{}, apperrors.New(apperrors.NodeAgentLoop, apperrors.CodeModelRequestFailed, "tool_call step missing tool_name")
		}
		if plan.Steps[i].Arguments == nil {
			plan.Steps[i].Arguments = map[string]any{}
		}
	}
	return plan, nil
}

// cleanJSONContent 去除模型可能包裹的 markdown 代码块标记，只保留 JSON 文本。
func cleanJSONContent(content string) string {
	cleaned := strings.TrimSpace(content)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	return strings.TrimSpace(cleaned)
}
