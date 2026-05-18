package eval

import (
	"fmt"

	tracing "mini-agent-runtime/internal/trace"
)

// traceCollector 收集单条 eval 执行期间的 trace 事件，供断言工具调用和工具错误。
type traceCollector struct {
	events []tracing.TraceEvent
}

// Emit 记录 trace 事件。
func (c *traceCollector) Emit(event tracing.TraceEvent) {
	c.events = append(c.events, event)
}

// ToolCalls 返回本次执行中观察到的工具调用。
func (c *traceCollector) ToolCalls() []ObservedToolCall {
	var calls []ObservedToolCall
	for _, event := range c.events {
		if event.Name != tracing.TraceToolCall {
			continue
		}
		data, ok := event.Data.(tracing.ToolCallTrace)
		if !ok {
			continue
		}
		calls = append(calls, ObservedToolCall{
			Name:      data.Name,
			Arguments: copyArguments(data.Arguments),
		})
	}
	return calls
}

// ToolResults 返回本次执行中观察到的工具结果。
func (c *traceCollector) ToolResults() []ObservedToolResult {
	var results []ObservedToolResult
	for _, event := range c.events {
		if event.Name != tracing.TraceToolResult {
			continue
		}
		data, ok := event.Data.(tracing.ToolResultTrace)
		if !ok {
			continue
		}
		results = append(results, ObservedToolResult{
			Name:   data.Name,
			Result: data.Result,
		})
	}
	return results
}

// ToolErrors 返回本次执行中观察到的工具错误。
func (c *traceCollector) ToolErrors() []ObservedToolError {
	var errors []ObservedToolError
	for _, event := range c.events {
		if event.Name != tracing.TraceToolError {
			continue
		}
		data, ok := event.Data.(tracing.ToolErrorTrace)
		if !ok {
			continue
		}
		errors = append(errors, ObservedToolError{
			Name:  data.Name,
			Error: fmt.Sprint(data.Error),
		})
	}
	return errors
}

// Observations 返回本次执行中观察到的 lifecycle observation。
func (c *traceCollector) Observations() []ObservedObservation {
	var observations []ObservedObservation
	for _, event := range c.events {
		if event.Name != tracing.TraceObservation {
			continue
		}
		data, ok := event.Data.(tracing.ObservationTrace)
		if !ok {
			continue
		}
		observations = append(observations, ObservedObservation{
			Type:    data.Type,
			Name:    data.Name,
			Content: data.Content,
			Error:   data.Error,
		})
	}
	return observations
}

// ApprovalRequests 返回本次执行中观察到的审批请求。
func (c *traceCollector) ApprovalRequests() []ObservedApprovalRequest {
	var requests []ObservedApprovalRequest
	for _, event := range c.events {
		if event.Name != tracing.TraceApprovalRequested {
			continue
		}
		data, ok := event.Data.(tracing.ApprovalRequestedTrace)
		if !ok {
			continue
		}
		requests = append(requests, ObservedApprovalRequest{
			ToolName:  data.ToolName,
			RiskLevel: data.RiskLevel,
			Reason:    data.Reason,
		})
	}
	return requests
}

// ApprovalDecisions 返回本次执行中观察到的审批决策。
func (c *traceCollector) ApprovalDecisions() []ObservedApprovalDecision {
	var decisions []ObservedApprovalDecision
	for _, event := range c.events {
		if event.Name != tracing.TraceApprovalDecided {
			continue
		}
		data, ok := event.Data.(tracing.ApprovalDecidedTrace)
		if !ok {
			continue
		}
		decisions = append(decisions, ObservedApprovalDecision{
			ToolName:  data.ToolName,
			RiskLevel: data.RiskLevel,
			Decision:  data.Decision,
			Approver:  data.Approver,
			Reason:    data.Reason,
		})
	}
	return decisions
}

// copyArguments 复制工具参数，避免后续断言修改 trace 中的原始 map。
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
