package approval

import (
	"context"
	"strings"
	"testing"
)

type lineReaderStub struct {
	line string
	err  error
}

// ReadLine 返回测试预设的一行输入。
func (r lineReaderStub) ReadLine(ctx context.Context) (string, error) {
	return r.line, r.err
}

// TestConsoleGateApprovesAcceptedInput 验证控制台 gate 接受明确确认输入。
func TestConsoleGateApprovesAcceptedInput(t *testing.T) {
	var output strings.Builder
	decision, err := NewConsoleGate(lineReaderStub{line: "确认"}, &output).RequestApproval(context.Background(), Request{
		ID:       "approval-1",
		ToolName: "dangerous_operation",
		RiskProfile: RiskProfile{
			Level:       RiskLevelHigh,
			Description: "模拟高危操作",
			Reasons:     []string{"高危操作需要人工确认"},
		},
		Arguments: map[string]any{"action": "删除生产数据"},
	})
	if err != nil {
		t.Fatalf("RequestApproval returned error: %v", err)
	}
	if got, want := decision.Status, StatusApproved; got != want {
		t.Fatalf("decision status = %q, want %q", got, want)
	}
	if !strings.Contains(output.String(), "风险等级: high") {
		t.Fatalf("output = %q, want risk level", output.String())
	}
}

// TestConsoleGateDeniesRejectedInput 验证控制台 gate 将非确认输入转换为拒绝决策。
func TestConsoleGateDeniesRejectedInput(t *testing.T) {
	var output strings.Builder
	decision, err := NewConsoleGate(lineReaderStub{line: "no"}, &output).RequestApproval(context.Background(), Request{
		ID:       "approval-1",
		ToolName: "dangerous_operation",
		RiskProfile: RiskProfile{
			Level: RiskLevelHigh,
		},
	})
	if err != nil {
		t.Fatalf("RequestApproval returned error: %v", err)
	}
	if got, want := decision.Status, StatusDenied; got != want {
		t.Fatalf("decision status = %q, want %q", got, want)
	}
	if !strings.Contains(output.String(), "已拒绝") {
		t.Fatalf("output = %q, want rejection message", output.String())
	}
}
