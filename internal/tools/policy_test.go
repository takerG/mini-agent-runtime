package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"mini-agent-runtime/internal/approval"
	"mini-agent-runtime/internal/ollama"
)

type retryTool struct {
	attempts int
}

// Name 返回重试测试工具的名称。
func (t *retryTool) Name() string {
	return "retry_tool"
}

// Description 返回重试测试工具的说明。
func (t *retryTool) Description() string {
	return "fail once then succeed"
}

// Definition 返回重试测试工具的模型可见定义。
func (t *retryTool) Definition() ollama.ToolDefinition {
	return ollama.ToolDefinition{Type: "function", Function: ollama.ToolDescription{Name: t.Name(), Description: t.Description()}}
}

// Execute 第一次调用失败，第二次调用成功，用于验证 retry policy。
func (t *retryTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	t.attempts++
	if t.attempts == 1 {
		return "", errors.New("temporary failure")
	}
	return "ok", nil
}

type slowTool struct{}

// Name 返回超时测试工具的名称。
func (t *slowTool) Name() string {
	return "slow_tool"
}

// Description 返回超时测试工具的说明。
func (t *slowTool) Description() string {
	return "wait until context is cancelled"
}

// Definition 返回超时测试工具的模型可见定义。
func (t *slowTool) Definition() ollama.ToolDefinition {
	return ollama.ToolDefinition{Type: "function", Function: ollama.ToolDescription{Name: t.Name(), Description: t.Description()}}
}

// Execute 阻塞直到 context 被取消，用于验证 timeout policy。
func (t *slowTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

type highRiskTestTool struct {
	executed bool
}

// Name 返回高风险测试工具的名称。
func (t *highRiskTestTool) Name() string {
	return "high_risk_test"
}

// Description 返回高风险测试工具的说明。
func (t *highRiskTestTool) Description() string {
	return "simulate a high-risk action"
}

// Definition 返回高风险测试工具的模型可见定义。
func (t *highRiskTestTool) Definition() ollama.ToolDefinition {
	return ollama.ToolDefinition{Type: "function", Function: ollama.ToolDescription{Name: t.Name(), Description: t.Description()}}
}

// RiskProfile 声明高风险测试工具必须经过人工确认。
func (t *highRiskTestTool) RiskProfile() approval.RiskProfile {
	return approval.RiskProfile{Level: approval.RiskLevelHigh}
}

// Execute 记录高风险测试工具是否真的被执行。
func (t *highRiskTestTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	t.executed = true
	return "executed", nil
}

type substringRetryPolicy struct {
	text string
}

// Retryable 判断错误文本是否包含指定片段。
func (p substringRetryPolicy) Retryable(err error) bool {
	return strings.Contains(err.Error(), p.text)
}

type denyAllPolicy struct {
	err error
}

// Allow 始终返回预设错误，用于验证工具准入拒绝逻辑。
func (p denyAllPolicy) Allow(ollama.ToolCall) error {
	return p.err
}

type approvalGateStub struct {
	err      error
	requests []approval.Request
}

// RequestApproval 记录审批请求，并按测试配置返回审批结果。
func (g *approvalGateStub) RequestApproval(ctx context.Context, request approval.Request) (approval.Decision, error) {
	g.requests = append(g.requests, request)
	return approval.Decision{RequestID: request.ID, Status: approval.StatusApproved}, g.err
}

// TestToolRegistryExecuteWithPolicyRetriesRetryableErrors 验证工具策略可以控制可重试错误的重试次数。
func TestToolRegistryExecuteWithPolicyRetriesRetryableErrors(t *testing.T) {
	registry := NewToolRegistry()
	tool := &retryTool{}
	registry.Register(tool)

	result, err := registry.ExecuteWithPolicy(context.Background(), ollama.ToolCall{
		Function: ollama.ToolFunctionCall{Name: "retry_tool"},
	}, ExecutionPolicy{
		MaxAttempts: 2,
		Retryable:   substringRetryPolicy{text: "temporary"},
	})
	if err != nil {
		t.Fatalf("ExecuteWithPolicy returned error: %v", err)
	}
	if got, want := result, "ok"; got != want {
		t.Fatalf("result = %q, want %q", got, want)
	}
	if got, want := tool.attempts, 2; got != want {
		t.Fatalf("attempts = %d, want %d", got, want)
	}
}

// TestToolRegistryExecuteWithPolicyTimesOutSlowTools 验证工具策略可以为单次工具执行设置超时。
func TestToolRegistryExecuteWithPolicyTimesOutSlowTools(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(&slowTool{})

	_, err := registry.ExecuteWithPolicy(context.Background(), ollama.ToolCall{
		Function: ollama.ToolFunctionCall{Name: "slow_tool"},
	}, ExecutionPolicy{Timeout: time.Millisecond})
	if err == nil {
		t.Fatal("ExecuteWithPolicy returned nil error, want timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %q, want timeout message", err.Error())
	}
}

// TestToolRegistryExecuteWithPolicyDeniesDisallowedTools 验证工具策略可以在执行前拒绝工具调用。
func TestToolRegistryExecuteWithPolicyDeniesDisallowedTools(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(&retryTool{})

	_, err := registry.ExecuteWithPolicy(context.Background(), ollama.ToolCall{
		Function: ollama.ToolFunctionCall{Name: "retry_tool"},
	}, ExecutionPolicy{
		Allow: denyAllPolicy{err: errors.New("approval required")},
	})
	if err == nil {
		t.Fatal("ExecuteWithPolicy returned nil error, want policy error")
	}
	if !strings.Contains(err.Error(), "approval required") {
		t.Fatalf("error = %q, want policy denial", err.Error())
	}
}

// TestToolRegistryRequiresApprovalForHighRiskTools 验证高风险工具没有人工确认时不会被执行。
func TestToolRegistryRequiresApprovalForHighRiskTools(t *testing.T) {
	registry := NewToolRegistry()
	tool := &highRiskTestTool{}
	registry.Register(tool)

	_, err := registry.ExecuteWithPolicy(context.Background(), ollama.ToolCall{
		Function: ollama.ToolFunctionCall{Name: "high_risk_test"},
	}, ExecutionPolicy{MaxAttempts: 1})
	if err == nil {
		t.Fatal("ExecuteWithPolicy returned nil error, want human approval error")
	}
	if !strings.Contains(err.Error(), "human approval required") {
		t.Fatalf("error = %q, want human approval message", err.Error())
	}
	if tool.executed {
		t.Fatal("high-risk tool executed without approval")
	}
}

// TestToolRegistryExecutesHighRiskToolAfterApproval 验证高风险工具获得人工确认后才会执行。
func TestToolRegistryExecutesHighRiskToolAfterApproval(t *testing.T) {
	registry := NewToolRegistry()
	tool := &highRiskTestTool{}
	registry.Register(tool)
	gate := &approvalGateStub{}

	result, err := registry.ExecuteWithPolicy(context.Background(), ollama.ToolCall{
		Function: ollama.ToolFunctionCall{
			Name: "high_risk_test",
			Arguments: map[string]any{
				"action": "delete everything",
			},
		},
	}, ExecutionPolicy{MaxAttempts: 1, Approval: approval.Policy{Gate: gate}})
	if err != nil {
		t.Fatalf("ExecuteWithPolicy returned error: %v", err)
	}
	if got, want := result, "executed"; got != want {
		t.Fatalf("result = %q, want %q", got, want)
	}
	if !tool.executed {
		t.Fatal("high-risk tool was not executed after approval")
	}
	if got, want := len(gate.requests), 1; got != want {
		t.Fatalf("approval requests = %d, want %d", got, want)
	}
	if got, want := gate.requests[0].ToolName, "high_risk_test"; got != want {
		t.Fatalf("approval tool name = %q, want %q", got, want)
	}
	if got, want := gate.requests[0].RiskProfile.Level, approval.RiskLevelHigh; got != want {
		t.Fatalf("approval risk level = %q, want %q", got, want)
	}
}
