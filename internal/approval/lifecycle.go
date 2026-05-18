package approval

import (
	"fmt"

	"mini-agent-runtime/internal/lifecycle"
	tracing "mini-agent-runtime/internal/trace"
)

// LifecycleRecorder 把审批过程同时写入 lifecycle observation 和 trace 事件。
type LifecycleRecorder struct {
	trace    *tracing.TraceHooks
	recorder *lifecycle.Recorder
	step     lifecycle.Step
}

// NewLifecycleRecorder 创建绑定到指定 tool step 的审批审计记录器。
func NewLifecycleRecorder(traceHooks *tracing.TraceHooks, recorder *lifecycle.Recorder, step lifecycle.Step) *LifecycleRecorder {
	return &LifecycleRecorder{trace: traceHooks, recorder: recorder, step: step}
}

// ApprovalRequested 记录审批请求事件。
func (r *LifecycleRecorder) ApprovalRequested(request Request) {
	if r == nil {
		return
	}
	r.emitRequested(request)
	r.addObservation(lifecycle.ObservationTypeApprovalRequest, request.ToolName, formatRequestObservation(request), nil)
}

// ApprovalDecided 记录审批决策事件。
func (r *LifecycleRecorder) ApprovalDecided(request Request, decision Decision) {
	if r == nil {
		return
	}
	r.emitDecided(request, decision)
	r.addObservation(lifecycle.ObservationTypeApprovalDecision, request.ToolName, formatDecisionObservation(decision), nil)
}

// ApprovalBypassed 记录无需审批的 bypass 事件。
func (r *LifecycleRecorder) ApprovalBypassed(request Request, decision Decision) {
	if r == nil {
		return
	}
	r.emitBypassed(request, decision)
}

// emitRequested 输出审批请求 trace 事件。
func (r *LifecycleRecorder) emitRequested(request Request) {
	if r.trace == nil {
		return
	}
	r.trace.ApprovalRequested(tracing.ApprovalRequestedTrace{
		RequestID: request.ID,
		ToolName:  request.ToolName,
		RiskLevel: string(request.RiskProfile.Level),
		Reason:    request.Reason,
	})
}

// emitDecided 输出审批决策 trace 事件。
func (r *LifecycleRecorder) emitDecided(request Request, decision Decision) {
	if r.trace == nil {
		return
	}
	r.trace.ApprovalDecided(tracing.ApprovalDecidedTrace{
		RequestID: request.ID,
		ToolName:  request.ToolName,
		RiskLevel: string(request.RiskProfile.Level),
		Decision:  string(decision.Status),
		Approver:  decision.Approver,
		Reason:    decision.Reason,
	})
}

// emitBypassed 输出审批 bypass trace 事件。
func (r *LifecycleRecorder) emitBypassed(request Request, decision Decision) {
	if r.trace == nil {
		return
	}
	r.trace.ApprovalBypassed(tracing.ApprovalBypassedTrace{
		RequestID: request.ID,
		ToolName:  request.ToolName,
		RiskLevel: string(request.RiskProfile.Level),
		Reason:    decision.Reason,
	})
}

// addObservation 记录审批相关 lifecycle observation，并同步输出 observation trace。
func (r *LifecycleRecorder) addObservation(observationType lifecycle.ObservationType, name string, content string, err error) {
	if r.recorder == nil || r.step.ID == "" {
		return
	}
	r.recorder.AddObservation(r.step.ID, observationType, name, content, err)
	if r.trace != nil {
		r.trace.Observation(tracing.ObservationTrace{
			Type:    string(observationType),
			Name:    name,
			Content: content,
			Error:   approvalErrorString(err),
		})
	}
}

// formatRequestObservation 格式化审批请求 observation 内容。
func formatRequestObservation(request Request) string {
	return fmt.Sprintf("approval requested: request_id=%s tool=%s risk=%s reason=%s", request.ID, request.ToolName, request.RiskProfile.Level, request.Reason)
}

// formatDecisionObservation 格式化审批决策 observation 内容。
func formatDecisionObservation(decision Decision) string {
	return fmt.Sprintf("approval decision: request_id=%s status=%s approver=%s reason=%s", decision.RequestID, decision.Status, decision.Approver, decision.Reason)
}

// approvalErrorString 把可选错误转换成 observation trace 里的稳定字符串。
func approvalErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
