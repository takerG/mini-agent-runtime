package tools

import (
	"context"
	"testing"
	"time"

	"mini-agent-runtime/internal/ollama"
)

func TestCurrentTimeToolImplementsToolInterface(t *testing.T) {
	var tool Tool = NewCurrentTimeTool(func() time.Time {
		return time.Date(2026, 5, 2, 18, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	})

	if got, want := tool.Name(), "current_time"; got != want {
		t.Fatalf("tool name = %q, want %q", got, want)
	}
	if tool.Description() == "" {
		t.Fatal("tool description is empty")
	}
	if got, want := tool.Definition().Function.Name, "current_time"; got != want {
		t.Fatalf("definition name = %q, want %q", got, want)
	}
}

func TestCurrentTimeToolFormatsInjectedTime(t *testing.T) {
	now := func() time.Time {
		return time.Date(2026, 4, 30, 17, 58, 9, 0, time.FixedZone("CST", 8*60*60))
	}

	got := CurrentTimeTool(now)
	want := "2026-04-30 17:58:09 CST"
	if got != want {
		t.Fatalf("CurrentTimeTool() = %q, want %q", got, want)
	}
}

func TestDefaultToolRegistryRunsCurrentTimeTool(t *testing.T) {
	registry := NewDefaultToolRegistry(func() time.Time {
		return time.Date(2026, 4, 30, 18, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	})

	got, err := registry.Execute(context.Background(), ollama.ToolCall{
		Function: ollama.ToolFunctionCall{
			Name:      "current_time",
			Arguments: map[string]any{},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if want := "2026-04-30 18:30:00 CST"; got != want {
		t.Fatalf("tool result = %q, want %q", got, want)
	}
}
