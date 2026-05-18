package tools

import (
	"context"
	"strings"
	"testing"

	"mini-agent-runtime/internal/approval"
	"mini-agent-runtime/internal/ollama"
)

// TestDangerousOperationToolRequiresHighRiskApproval 验证模拟高危工具声明为高风险工具。
func TestDangerousOperationToolRequiresHighRiskApproval(t *testing.T) {
	tool := NewDangerousOperationTool()
	riskAware, ok := tool.(RiskAwareTool)
	if !ok {
		t.Fatal("dangerous operation tool does not implement RiskAwareTool")
	}
	if got, want := riskAware.RiskProfile().Level, approval.RiskLevelHigh; got != want {
		t.Fatalf("risk level = %q, want %q", got, want)
	}
}

// TestDangerousOperationToolPrintsHighRiskMarker 验证模拟高危工具只返回高危操作标记，不执行真实副作用。
func TestDangerousOperationToolPrintsHighRiskMarker(t *testing.T) {
	result, err := NewDangerousOperationTool().Execute(context.Background(), map[string]any{
		"action": "删除生产数据",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(result, "【高危操作】") {
		t.Fatalf("result = %q, want high-risk marker", result)
	}
	if !strings.Contains(result, "删除生产数据") {
		t.Fatalf("result = %q, want action description", result)
	}
}

// TestNewDefaultToolRegistryIncludesDangerousOperation 验证默认注册表包含用于验收 HITL 的模拟高危工具。
func TestNewDefaultToolRegistryIncludesDangerousOperation(t *testing.T) {
	registry := NewDefaultToolRegistry(nil)
	result, err := registry.ExecuteWithPolicy(context.Background(), ollama.ToolCall{
		Function: ollama.ToolFunctionCall{
			Name: "dangerous_operation",
			Arguments: map[string]any{
				"action": "删除生产数据",
			},
		},
	}, ExecutionPolicy{MaxAttempts: 1, Approval: approval.Policy{Gate: &approvalGateStub{}}})
	if err != nil {
		t.Fatalf("ExecuteWithPolicy returned error: %v", err)
	}
	if !strings.Contains(result, "【高危操作】") {
		t.Fatalf("result = %q, want high-risk marker", result)
	}
}
