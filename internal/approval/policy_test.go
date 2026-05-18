package approval

import (
	"context"
	"strings"
	"testing"
	"time"
)

type recordingRecorder struct {
	requests  []Request
	decisions []Decision
	bypassed  []Decision
}

// ApprovalRequested 记录测试中的审批请求。
func (r *recordingRecorder) ApprovalRequested(request Request) {
	r.requests = append(r.requests, request)
}

// ApprovalDecided 记录测试中的审批决策。
func (r *recordingRecorder) ApprovalDecided(request Request, decision Decision) {
	r.decisions = append(r.decisions, decision)
}

// ApprovalBypassed 记录测试中的审批跳过决策。
func (r *recordingRecorder) ApprovalBypassed(request Request, decision Decision) {
	r.bypassed = append(r.bypassed, decision)
}

type gateStub struct {
	decision Decision
	err      error
	requests []Request
}

// RequestApproval 记录请求并返回预设决策。
func (g *gateStub) RequestApproval(ctx context.Context, request Request) (Decision, error) {
	g.requests = append(g.requests, request)
	return g.decision, g.err
}

// TestPolicyBypassesLowRiskTools 验证低风险工具不会触发人工审批。
func TestPolicyBypassesLowRiskTools(t *testing.T) {
	recorder := &recordingRecorder{}
	decision, err := fixedPolicy(recorder, nil).Review(context.Background(), Evaluation{
		ToolName: "calculator",
		RiskProfile: RiskProfile{
			Level:       RiskLevelLow,
			Description: "safe calculation",
		},
	})
	if err != nil {
		t.Fatalf("Review returned error: %v", err)
	}
	if got, want := decision.Status, StatusBypassed; got != want {
		t.Fatalf("decision status = %q, want %q", got, want)
	}
	if got, want := len(recorder.bypassed), 1; got != want {
		t.Fatalf("bypassed decisions = %d, want %d", got, want)
	}
	if got := len(recorder.requests); got != 0 {
		t.Fatalf("approval requests = %d, want 0", got)
	}
}

// TestPolicyRejectsHighRiskToolsWithoutGate 验证高风险工具没有审批 gate 时不会被放行。
func TestPolicyRejectsHighRiskToolsWithoutGate(t *testing.T) {
	recorder := &recordingRecorder{}
	decision, err := fixedPolicy(recorder, nil).Review(context.Background(), Evaluation{
		ToolName: "dangerous_operation",
		RiskProfile: RiskProfile{
			Level:       RiskLevelHigh,
			Description: "delete production data",
		},
	})
	if err == nil {
		t.Fatal("Review returned nil error, want approval error")
	}
	if !strings.Contains(err.Error(), "human approval required") {
		t.Fatalf("error = %q, want approval required message", err.Error())
	}
	if got, want := decision.Status, StatusDenied; got != want {
		t.Fatalf("decision status = %q, want %q", got, want)
	}
	if got, want := len(recorder.requests), 1; got != want {
		t.Fatalf("approval requests = %d, want %d", got, want)
	}
	if got, want := len(recorder.decisions), 1; got != want {
		t.Fatalf("approval decisions = %d, want %d", got, want)
	}
}

// TestPolicyApprovesHighRiskToolsThroughGate 验证高风险工具经过 gate 批准后可以继续执行。
func TestPolicyApprovesHighRiskToolsThroughGate(t *testing.T) {
	recorder := &recordingRecorder{}
	gate := &gateStub{
		decision: Decision{
			Status:   StatusApproved,
			Approver: "tester",
			Reason:   "approved in test",
		},
	}
	decision, err := fixedPolicy(recorder, gate).Review(context.Background(), Evaluation{
		ToolName: "dangerous_operation",
		Arguments: map[string]any{
			"action": "删除生产数据",
		},
		RiskProfile: RiskProfile{
			Level:       RiskLevelHigh,
			Category:    "demo",
			Description: "simulate dangerous action",
			Reasons:     []string{"高危操作需要人工确认"},
		},
	})
	if err != nil {
		t.Fatalf("Review returned error: %v", err)
	}
	if got, want := decision.Status, StatusApproved; got != want {
		t.Fatalf("decision status = %q, want %q", got, want)
	}
	if got, want := gate.requests[0].ID, "approval-fixed"; got != want {
		t.Fatalf("request id = %q, want %q", got, want)
	}
	if got, want := gate.requests[0].Arguments["action"], "删除生产数据"; got != want {
		t.Fatalf("request argument = %v, want %v", got, want)
	}
	if got, want := len(recorder.decisions), 1; got != want {
		t.Fatalf("approval decisions = %d, want %d", got, want)
	}
}

// fixedPolicy 返回带固定时间和 ID 的审批策略，便于测试稳定断言。
func fixedPolicy(recorder Recorder, gate Gate) Policy {
	return Policy{
		Gate:     gate,
		Recorder: recorder,
		Clock: func() time.Time {
			return time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
		},
		IDGenerator: func() string {
			return "approval-fixed"
		},
	}
}
