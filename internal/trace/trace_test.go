package trace

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"mini-agent-runtime/internal/ollama"
)

// TestTraceLoggerWritesOnlyWhenEnabled 验证 trace logger 只在启用时写日志。
func TestTraceLoggerWritesOnlyWhenEnabled(t *testing.T) {
	var enabled strings.Builder
	trace := NewTraceLogger(true, &enabled)
	trace.Log("step", "value=%s", "one")
	if got, want := enabled.String(), "[trace] step: value=one\n"; got != want {
		t.Fatalf("enabled trace = %q, want %q", got, want)
	}

	var disabled strings.Builder
	trace = NewTraceLogger(false, &disabled)
	trace.Log("step", "value=%s", "two")
	if got := disabled.String(); got != "" {
		t.Fatalf("disabled trace = %q, want empty", got)
	}
}

// TestTraceHooksEmitStructuredEventsToSink 验证 trace hooks 会把结构化事件发送给 sink。
func TestTraceHooksEmitStructuredEventsToSink(t *testing.T) {
	sink := &recordingTraceSink{}
	hooks := NewTraceHooks(sink)

	hooks.ChatLoopStart(ChatLoopStartTrace{
		Endpoint: "http://localhost:11434/api/chat",
		Model:    "qwen3:4b",
		Think:    true,
		Tools:    2,
	})
	hooks.ToolResult(ToolResultTrace{
		Name:   "calculator",
		Result: "5",
	})

	if got, want := len(sink.events), 2; got != want {
		t.Fatalf("event count = %d, want %d", got, want)
	}
	if got, want := sink.events[0].Name, TraceChatLoopStart; got != want {
		t.Fatalf("first event name = %q, want %q", got, want)
	}
	if got, want := sink.events[1].Name, TraceToolResult; got != want {
		t.Fatalf("second event name = %q, want %q", got, want)
	}
}

// TestTraceLoggerIncludesFullModelRequestAndResponse 验证 trace 日志包含完整模型请求和响应。
func TestTraceLoggerIncludesFullModelRequestAndResponse(t *testing.T) {
	var output strings.Builder
	logger := NewTraceLogger(true, &output)
	think := true

	logger.Emit(TraceEvent{
		Name: TraceModelRequest,
		Data: ModelRequestTrace{
			Phase:     "chat",
			ToolRound: 1,
			Request: ollama.ChatRequest{
				Model:  "qwen3:4b",
				Stream: true,
				Think:  &think,
				Messages: []ollama.Message{
					{Role: "user", Content: "2+3 等于多少？"},
				},
				Tools: []ollama.ToolDefinition{
					{
						Type: "function",
						Function: ollama.ToolDescription{
							Name:        "calculator",
							Description: "run calculation",
						},
					},
				},
			},
		},
	})
	logger.Emit(TraceEvent{
		Name: TraceModelResponse,
		Data: ModelResponseTrace{
			Phase:     "chat",
			ToolRound: 1,
			Content:   "2+3=5",
			ToolCalls: []ollama.ToolCall{
				{Function: ollama.ToolFunctionCall{Name: "calculator", Arguments: map[string]any{"a": 2, "b": 3, "op": "+"}}},
			},
		},
	})

	got := output.String()
	for _, want := range []string{
		`"model": "qwen3:4b"`,
		`"content": "2+3 等于多少？"`,
		`"name": "calculator"`,
		`"content": "2+3=5"`,
		`"tool_calls"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("trace output = %q, want to contain %s", got, want)
		}
	}
}

// TestTraceHooksAttachRunAndStepContext 验证 trace hooks 会把运行上下文挂到所有派生事件上。
func TestTraceHooksAttachRunAndStepContext(t *testing.T) {
	sink := &recordingTraceSink{}
	hooks := NewTraceHooks(sink).WithContext(TraceContext{
		RunID:        "run-000001",
		StepID:       "step-000002",
		ParentStepID: "step-000001",
	})

	hooks.ToolResult(ToolResultTrace{Name: "calculator", Result: "437"})

	if got, want := sink.events[0].RunID, "run-000001"; got != want {
		t.Fatalf("run id = %q, want %q", got, want)
	}
	if got, want := sink.events[0].StepID, "step-000002"; got != want {
		t.Fatalf("step id = %q, want %q", got, want)
	}
	if got, want := sink.events[0].ParentStepID, "step-000001"; got != want {
		t.Fatalf("parent step id = %q, want %q", got, want)
	}
}

// TestTraceHooksEmitApprovalEvents 验证 trace hooks 会输出审批请求和审批决策事件。
func TestTraceHooksEmitApprovalEvents(t *testing.T) {
	sink := &recordingTraceSink{}
	hooks := NewTraceHooks(sink).WithContext(TraceContext{
		RunID:  "run-000001",
		StepID: "step-000002",
	})

	hooks.ApprovalRequested(ApprovalRequestedTrace{
		RequestID: "approval-1",
		ToolName:  "dangerous_operation",
		RiskLevel: "high",
		Reason:    "高危操作需要人工确认",
	})
	hooks.ApprovalDecided(ApprovalDecidedTrace{
		RequestID: "approval-1",
		ToolName:  "dangerous_operation",
		RiskLevel: "high",
		Decision:  "approved",
		Approver:  "cli",
	})

	if got, want := len(sink.events), 2; got != want {
		t.Fatalf("event count = %d, want %d", got, want)
	}
	if got, want := sink.events[0].Name, TraceApprovalRequested; got != want {
		t.Fatalf("first event name = %q, want %q", got, want)
	}
	if got, want := sink.events[1].Name, TraceApprovalDecided; got != want {
		t.Fatalf("second event name = %q, want %q", got, want)
	}
}

// TestTraceJSONLLoggerWritesStructuredEvents 验证 JSONL trace sink 会输出可被逐行解析的结构化事件。
func TestTraceJSONLLoggerWritesStructuredEvents(t *testing.T) {
	var output strings.Builder
	logger := NewTraceJSONLLoggerWithClock(true, &output, func() time.Time {
		return time.Date(2026, 5, 14, 10, 30, 0, 0, time.UTC)
	})

	logger.Emit(TraceEvent{
		Name:   TraceToolResult,
		RunID:  "run-000001",
		StepID: "step-000002",
		Data:   ToolResultTrace{Name: "calculator", Result: "437"},
	})

	var event map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output.String())), &event); err != nil {
		t.Fatalf("jsonl event is not valid json: %v", err)
	}
	if got, want := event["name"], string(TraceToolResult); got != want {
		t.Fatalf("jsonl name = %q, want %q", got, want)
	}
	if got, want := event["run_id"], "run-000001"; got != want {
		t.Fatalf("jsonl run_id = %q, want %q", got, want)
	}
	if got, want := event["step_id"], "step-000002"; got != want {
		t.Fatalf("jsonl step_id = %q, want %q", got, want)
	}
	if got, want := event["timestamp"], "2026-05-14T10:30:00Z"; got != want {
		t.Fatalf("jsonl timestamp = %q, want %q", got, want)
	}
	data := event["data"].(map[string]any)
	if got, want := data["result"], "437"; got != want {
		t.Fatalf("jsonl data result = %q, want %q", got, want)
	}
}

type recordingTraceSink struct {
	events []TraceEvent
}

// Emit 把 trace 事件追加到测试 sink 中，便于断言。
func (s *recordingTraceSink) Emit(event TraceEvent) {
	s.events = append(s.events, event)
}
