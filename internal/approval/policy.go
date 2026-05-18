package approval

import (
	"context"
	"fmt"
	"time"

	apperrors "mini-agent-runtime/internal/errors"
)

// IDGenerator 定义审批请求 ID 的生成函数。
type IDGenerator func() string

// Clock 定义审批系统使用的时间来源。
type Clock func() time.Time

// Recorder 定义审批请求和决策的审计记录接口。
type Recorder interface {
	ApprovalRequested(request Request)
	ApprovalDecided(request Request, decision Decision)
	ApprovalBypassed(request Request, decision Decision)
}

// Policy 描述工具执行前的审批治理策略。
type Policy struct {
	Gate        Gate
	Recorder    Recorder
	Context     RuntimeContext
	Clock       Clock
	IDGenerator IDGenerator
	TTL         time.Duration
}

// Review 根据风险画像判断工具调用是否可以继续执行。
func (p Policy) Review(ctx context.Context, evaluation Evaluation) (Decision, error) {
	p = p.normalized()
	request := p.newRequest(evaluation)
	if !requiresApproval(request.RiskProfile.Level) {
		decision := p.newDecision(request, StatusBypassed, "runtime", "risk level does not require approval")
		if p.Recorder != nil {
			p.Recorder.ApprovalBypassed(request, decision)
		}
		return decision, nil
	}

	if p.Recorder != nil {
		p.Recorder.ApprovalRequested(request)
	}
	if p.Gate == nil {
		decision := p.newDecision(request, StatusDenied, "runtime", "human approval gate is not configured")
		if p.Recorder != nil {
			p.Recorder.ApprovalDecided(request, decision)
		}
		return decision, apperrors.New(apperrors.NodeApproval, apperrors.CodeHumanApprovalRequired, fmt.Sprintf("human approval required for high-risk tool: %s", request.ToolName))
	}

	decision, err := p.Gate.RequestApproval(ctx, request)
	decision = p.normalizeDecision(request, decision)
	if p.Recorder != nil {
		p.Recorder.ApprovalDecided(request, decision)
	}
	if err != nil {
		return decision, apperrors.Wrap(apperrors.NodeApproval, apperrors.CodeHumanApprovalDenied, err, fmt.Sprintf("human approval failed for high-risk tool: %s", request.ToolName))
	}
	if !decision.Approved() {
		return decision, p.deniedError(request, decision)
	}
	return decision, nil
}

// normalized 补齐审批策略的默认时钟、ID 生成器和有效期。
func (p Policy) normalized() Policy {
	if p.Clock == nil {
		p.Clock = time.Now
	}
	if p.IDGenerator == nil {
		p.IDGenerator = defaultIDGenerator
	}
	if p.TTL <= 0 {
		p.TTL = 5 * time.Minute
	}
	return p
}

// newRequest 根据审批评估输入创建稳定的审批请求。
func (p Policy) newRequest(evaluation Evaluation) Request {
	now := p.Clock()
	profile := evaluation.RiskProfile.Normalized()
	return Request{
		ID:           p.IDGenerator(),
		RunID:        p.Context.RunID,
		StepID:       p.Context.StepID,
		ParentStepID: p.Context.ParentStepID,
		ToolName:     evaluation.ToolName,
		Description:  evaluation.Description,
		Arguments:    copyArguments(evaluation.Arguments),
		RiskProfile:  profile,
		Reason:       approvalReason(profile),
		CreatedAt:    now,
		ExpiresAt:    now.Add(p.TTL),
	}
}

// newDecision 创建带默认时间的审批决策。
func (p Policy) newDecision(request Request, status Status, approver string, reason string) Decision {
	return Decision{
		RequestID: request.ID,
		Status:    status,
		Approver:  approver,
		Reason:    reason,
		DecidedAt: p.Clock(),
	}
}

// normalizeDecision 补齐 gate 返回决策中的请求 ID、状态和时间。
func (p Policy) normalizeDecision(request Request, decision Decision) Decision {
	if decision.RequestID == "" {
		decision.RequestID = request.ID
	}
	if decision.Status == "" {
		decision.Status = StatusDenied
	}
	if decision.DecidedAt.IsZero() {
		decision.DecidedAt = p.Clock()
	}
	return decision
}

// deniedError 根据未批准决策返回稳定错误。
func (p Policy) deniedError(request Request, decision Decision) error {
	if decision.Status == StatusExpired {
		return apperrors.New(apperrors.NodeApproval, apperrors.CodeHumanApprovalExpired, fmt.Sprintf("human approval expired for high-risk tool: %s", request.ToolName))
	}
	return apperrors.New(apperrors.NodeApproval, apperrors.CodeHumanApprovalDenied, fmt.Sprintf("human approval denied for high-risk tool: %s reason=%s", request.ToolName, decision.Reason))
}

// requiresApproval 判断指定风险等级是否需要审批。
func requiresApproval(level RiskLevel) bool {
	return level == RiskLevelHigh || level == RiskLevelCritical
}

// approvalReason 根据风险画像生成审批原因摘要。
func approvalReason(profile RiskProfile) string {
	if len(profile.Reasons) > 0 {
		return profile.Reasons[0]
	}
	if profile.Description != "" {
		return profile.Description
	}
	return fmt.Sprintf("risk level %s requires governance", profile.Level)
}

// copyArguments 复制审批请求参数，避免审批方修改原始工具参数。
func copyArguments(arguments map[string]any) map[string]any {
	if arguments == nil {
		return nil
	}
	copied := make(map[string]any, len(arguments))
	for key, value := range arguments {
		copied[key] = value
	}
	return copied
}

// defaultIDGenerator 生成默认审批请求 ID。
func defaultIDGenerator() string {
	return fmt.Sprintf("approval-%d", time.Now().UnixNano())
}
