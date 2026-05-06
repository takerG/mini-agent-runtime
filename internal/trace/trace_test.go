package trace

import (
	"strings"
	"testing"
)

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

type recordingTraceSink struct {
	events []TraceEvent
}

func (s *recordingTraceSink) Emit(event TraceEvent) {
	s.events = append(s.events, event)
}
