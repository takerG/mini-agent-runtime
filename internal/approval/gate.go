package approval

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// LineReader 定义人工确认器读取单行输入的能力。
type LineReader interface {
	// ReadLine 读取一行用户输入，返回值不包含结尾换行。
	ReadLine(ctx context.Context) (string, error)
}

// ConsoleGate 通过命令行提示用户确认高风险工具调用。
type ConsoleGate struct {
	reader LineReader
	writer io.Writer
}

// NewConsoleGate 创建基于命令行输入输出的人工确认器。
func NewConsoleGate(reader LineReader, writer io.Writer) *ConsoleGate {
	if writer == nil {
		writer = io.Discard
	}
	return &ConsoleGate{reader: reader, writer: writer}
}

// RequestApproval 输出高风险工具提示，并把用户输入转换成审批决策。
func (g *ConsoleGate) RequestApproval(ctx context.Context, request Request) (Decision, error) {
	if g == nil || g.reader == nil {
		return Decision{RequestID: request.ID, Status: StatusDenied, Reason: "approval reader is not configured"}, fmt.Errorf("human approval reader is not configured")
	}
	g.writePrompt(request)
	line, err := g.reader.ReadLine(ctx)
	if err != nil {
		return Decision{RequestID: request.ID, Status: StatusDenied, Reason: "read human approval failed"}, fmt.Errorf("read human approval: %w", err)
	}
	if isAcceptedInput(line) {
		_, _ = fmt.Fprintln(g.writer, "[approval] 已确认，继续执行。")
		return Decision{
			RequestID: request.ID,
			Status:    StatusApproved,
			Approver:  "cli",
			Reason:    "approved by console input",
			DecidedAt: time.Now(),
		}, nil
	}
	_, _ = fmt.Fprintln(g.writer, "[approval] 已拒绝，本次工具不会执行。")
	return Decision{
		RequestID: request.ID,
		Status:    StatusDenied,
		Approver:  "cli",
		Reason:    fmt.Sprintf("human rejected high-risk tool: %s", request.ToolName),
		DecidedAt: time.Now(),
	}, nil
}

// writePrompt 把审批请求格式化输出到命令行。
func (g *ConsoleGate) writePrompt(request Request) {
	_, _ = fmt.Fprintf(g.writer, "[approval] 高风险工具调用: %s\n", request.ToolName)
	_, _ = fmt.Fprintf(g.writer, "[approval] 风险等级: %s\n", request.RiskProfile.Level)
	if request.RiskProfile.Category != "" {
		_, _ = fmt.Fprintf(g.writer, "[approval] 风险类别: %s\n", request.RiskProfile.Category)
	}
	if request.RiskProfile.Description != "" {
		_, _ = fmt.Fprintf(g.writer, "[approval] 说明: %s\n", request.RiskProfile.Description)
	}
	if len(request.RiskProfile.Reasons) > 0 {
		_, _ = fmt.Fprintf(g.writer, "[approval] 原因: %s\n", strings.Join(request.RiskProfile.Reasons, "；"))
	}
	if len(request.Arguments) > 0 {
		_, _ = fmt.Fprintf(g.writer, "[approval] 参数: %v\n", request.Arguments)
	}
	_, _ = fmt.Fprint(g.writer, "[approval] 输入 yes/y/confirm/确认 执行，输入 no/n/拒绝 或其他内容取消: ")
}

// isAcceptedInput 判断用户输入是否表示确认高风险操作。
func isAcceptedInput(line string) bool {
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes", "确认", "confirm":
		return true
	default:
		return false
	}
}

// AutoGate 是测试或自动化场景使用的固定决策 gate。
type AutoGate struct {
	decision Decision
}

// NewAutoGate 创建始终返回指定决策的自动 gate。
func NewAutoGate(decision Decision) *AutoGate {
	return &AutoGate{decision: decision}
}

// RequestApproval 返回自动 gate 的固定决策。
func (g *AutoGate) RequestApproval(ctx context.Context, request Request) (Decision, error) {
	decision := g.decision
	if decision.RequestID == "" {
		decision.RequestID = request.ID
	}
	if decision.Status == "" {
		decision.Status = StatusApproved
	}
	if decision.Approver == "" {
		decision.Approver = "auto"
	}
	if decision.DecidedAt.IsZero() {
		decision.DecidedAt = time.Now()
	}
	return decision, nil
}
