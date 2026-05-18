package approval

import (
	"context"
	"time"
)

// RiskLevel 表示工具调用在执行前需要的安全治理等级。
type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

// RiskProfile 描述工具调用的风险画像，供审批策略和审计展示使用。
type RiskProfile struct {
	Level       RiskLevel
	Category    string
	Description string
	Reasons     []string
}

// Normalized 返回补齐默认风险等级后的风险画像。
func (p RiskProfile) Normalized() RiskProfile {
	if p.Level == "" {
		p.Level = RiskLevelLow
	}
	return p
}

// RuntimeContext 描述审批请求所在的 agent run 和 step 上下文。
type RuntimeContext struct {
	RunID        string
	StepID       string
	ParentStepID string
}

// Request 描述一次提交给人工或自动 gate 的审批请求。
type Request struct {
	ID          string
	RunID       string
	StepID      string
	ParentStepID string
	ToolName    string
	Description string
	Arguments   map[string]any
	RiskProfile RiskProfile
	Reason      string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

// Status 表示审批请求最终得到的治理决策。
type Status string

const (
	StatusApproved  Status = "approved"
	StatusDenied    Status = "denied"
	StatusExpired   Status = "expired"
	StatusCancelled Status = "cancelled"
	StatusBypassed  Status = "bypassed"
)

// Decision 表示一次审批请求的决策结果。
type Decision struct {
	RequestID string
	Status    Status
	Approver  string
	Reason    string
	DecidedAt time.Time
}

// Approved 判断审批结果是否允许工具继续执行。
func (d Decision) Approved() bool {
	return d.Status == StatusApproved || d.Status == StatusBypassed
}

// Evaluation 描述审批策略评估一次工具调用时需要的输入。
type Evaluation struct {
	RuntimeContext RuntimeContext
	ToolName       string
	Description    string
	Arguments      map[string]any
	RiskProfile    RiskProfile
}

// Gate 定义高风险工具调用前的人工或外部系统审批接口。
type Gate interface {
	// RequestApproval 提交审批请求，并返回审批决策。
	RequestApproval(ctx context.Context, request Request) (Decision, error)
}
