package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"mini-agent-runtime/internal/agent"
)

// Suite 表示一个 eval JSON 文件中的完整测试集合。
type Suite struct {
	Name  string `json:"name"`
	Cases []Case `json:"cases"`
}

// Case 表示单条 eval 用例，支持指定单个 mode 或多个 modes。
type Case struct {
	Name   string      `json:"name"`
	Input  string      `json:"input"`
	Mode   agent.Mode  `json:"mode,omitempty"`
	Modes  []agent.Mode `json:"modes,omitempty"`
	Expect Expectation `json:"expect"`
}

// Expectation 描述 eval 用例对工具调用、最终答案和错误行为的期望。
type Expectation struct {
	ToolCalls         []ExpectedToolCall  `json:"tool_calls,omitempty"`
	ToolCallCount     *int                `json:"tool_call_count,omitempty"`
	ToolResults       []ExpectedToolResult `json:"tool_results,omitempty"`
	Observations      []ExpectedObservation `json:"observations,omitempty"`
	ApprovalRequests  []ExpectedApprovalRequest `json:"approval_requests,omitempty"`
	ApprovalDecisions []ExpectedApprovalDecision `json:"approval_decisions,omitempty"`
	ToolErrors        []ExpectedToolError `json:"tool_errors,omitempty"`
	AnswerContains    []string            `json:"answer_contains,omitempty"`
	AnswerNotContains []string            `json:"answer_not_contains,omitempty"`
	ExpectError       bool                `json:"expect_error,omitempty"`
	ErrorContains     []string            `json:"error_contains,omitempty"`
}

// ExpectedToolCall 描述期望出现的工具调用。
type ExpectedToolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// ExpectedToolResult 描述期望出现的结构化工具结果。
type ExpectedToolResult struct {
	Name              string   `json:"name,omitempty"`
	ResultContains    []string `json:"result_contains,omitempty"`
	ResultNotContains []string `json:"result_not_contains,omitempty"`
}

// ExpectedObservation 描述期望出现的结构化 lifecycle observation。
type ExpectedObservation struct {
	Type               string   `json:"type,omitempty"`
	Name               string   `json:"name,omitempty"`
	ContentContains    []string `json:"content_contains,omitempty"`
	ContentNotContains []string `json:"content_not_contains,omitempty"`
	ErrorContains      []string `json:"error_contains,omitempty"`
}

// ExpectedApprovalRequest 描述期望出现的审批请求。
type ExpectedApprovalRequest struct {
	ToolName       string   `json:"tool_name,omitempty"`
	RiskLevel      string   `json:"risk_level,omitempty"`
	ReasonContains []string `json:"reason_contains,omitempty"`
}

// ExpectedApprovalDecision 描述期望出现的审批决策。
type ExpectedApprovalDecision struct {
	ToolName       string   `json:"tool_name,omitempty"`
	RiskLevel      string   `json:"risk_level,omitempty"`
	Decision       string   `json:"decision,omitempty"`
	Approver       string   `json:"approver,omitempty"`
	ReasonContains []string `json:"reason_contains,omitempty"`
}

// ExpectedToolError 描述期望出现的工具错误。
type ExpectedToolError struct {
	Name          string   `json:"name,omitempty"`
	ErrorContains []string `json:"error_contains,omitempty"`
}

// LoadSuite 从 JSON 文件读取 eval suite，并补齐默认 suite 名称。
func LoadSuite(path string) (Suite, error) {
	file, err := os.Open(path)
	if err != nil {
		return Suite{}, fmt.Errorf("open eval suite: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	var suite Suite
	if err := json.NewDecoder(file).Decode(&suite); err != nil {
		return Suite{}, fmt.Errorf("decode eval suite: %w", err)
	}
	if suite.Name == "" {
		suite.Name = filepath.Base(path)
	}
	if err := suite.Validate(); err != nil {
		return Suite{}, err
	}
	return suite, nil
}

// Validate 检查 suite 中的必填字段和 mode 配置。
func (s Suite) Validate() error {
	if len(s.Cases) == 0 {
		return fmt.Errorf("eval suite %q has no cases", s.Name)
	}
	for index, evalCase := range s.Cases {
		if evalCase.Name == "" {
			return fmt.Errorf("eval case[%d] missing name", index)
		}
		if evalCase.Input == "" {
			return fmt.Errorf("eval case %q missing input", evalCase.Name)
		}
		if evalCase.Mode != "" && len(evalCase.Modes) > 0 {
			return fmt.Errorf("eval case %q cannot set both mode and modes", evalCase.Name)
		}
		for _, mode := range evalCase.TargetModes() {
			if !isSupportedMode(mode) {
				return fmt.Errorf("eval case %q has unsupported mode: %s", evalCase.Name, mode)
			}
		}
	}
	return nil
}

// TargetModes 返回用例需要运行的模式；未声明时默认覆盖全部 agent 模式。
func (c Case) TargetModes() []agent.Mode {
	if c.Mode != "" {
		return []agent.Mode{c.Mode}
	}
	if len(c.Modes) > 0 {
		return append([]agent.Mode(nil), c.Modes...)
	}
	return []agent.Mode{agent.ModeChat, agent.ModePlan, agent.ModeStrictPlan}
}

// isSupportedMode 判断 eval runner 是否支持指定 agent mode。
func isSupportedMode(mode agent.Mode) bool {
	switch mode {
	case agent.ModeChat, agent.ModePlan, agent.ModeStrictPlan:
		return true
	default:
		return false
	}
}
