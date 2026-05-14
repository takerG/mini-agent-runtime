package executor

import (
	"context"
	"strings"
	"testing"
	"time"

	"mini-agent-runtime/internal/planner"
	"mini-agent-runtime/internal/tools"
	tracing "mini-agent-runtime/internal/trace"
)

// TestStrictExecutorRunsExecutablePlanAndPrintsProcess 验证 strict executor 会直接执行可执行计划并展示过程。
func TestStrictExecutorRunsExecutablePlanAndPrintsProcess(t *testing.T) {
	var stdout strings.Builder
	executor := NewStrictExecutor(StrictExecutorOptions{
		Dependencies: Dependencies{
			Registry: tools.NewDefaultToolRegistry(func() time.Time {
				return time.Date(2026, 5, 14, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60))
			}),
			Trace:  tracing.NewTraceHooks(nil),
			Stdout: &stdout,
		},
		ShowProcess: true,
	})

	observations := executor.Execute(context.Background(), planner.ExecutablePlan{
		Goal: "make record",
		Steps: []planner.ExecutableStep{
			{Type: planner.ExecutableStepToolCall, ToolName: "calculator", Arguments: map[string]any{"a": float64(23), "b": float64(19), "op": "*"}},
			{Type: planner.ExecutableStepToolCall, ToolName: "current_time", Arguments: map[string]any{}},
		},
	})

	if got, want := len(observations), 2; got != want {
		t.Fatalf("observation count = %d, want %d", got, want)
	}
	if got, want := observations[0], (StrictObservation{ToolName: "calculator", Result: "437"}); got != want {
		t.Fatalf("first observation = %#v, want %#v", got, want)
	}
	if got, want := observations[1], (StrictObservation{ToolName: "current_time", Result: "2026-05-14 09:30:00 CST"}); got != want {
		t.Fatalf("second observation = %#v, want %#v", got, want)
	}

	wantOutput := strings.Join([]string{
		"[plan]",
		`1. tool_call calculator {"a":23,"b":19,"op":"*"}`,
		"2. tool_call current_time {}",
		"",
		"[observation]",
		"1. calculator -> 437",
		"2. current_time -> 2026-05-14 09:30:00 CST",
		"",
		"Agent:",
		"",
	}, "\n")
	if got := stdout.String(); got != wantOutput {
		t.Fatalf("stdout = %q, want %q", got, wantOutput)
	}
}

// TestStrictExecutorFeedsToolErrorsIntoObservations 验证 strict executor 不会因工具错误退出，而是把错误转成 observation。
func TestStrictExecutorFeedsToolErrorsIntoObservations(t *testing.T) {
	var stdout strings.Builder
	executor := NewStrictExecutor(StrictExecutorOptions{
		Dependencies: Dependencies{
			Registry: tools.NewDefaultToolRegistry(nil),
			Trace:    tracing.NewTraceHooks(nil),
			Stdout:   &stdout,
		},
		ShowProcess: true,
	})

	observations := executor.Execute(context.Background(), planner.ExecutablePlan{
		Goal: "bad tool",
		Steps: []planner.ExecutableStep{
			{Type: planner.ExecutableStepToolCall, ToolName: "missing_tool", Arguments: map[string]any{}},
		},
	})

	if got, want := len(observations), 1; got != want {
		t.Fatalf("observation count = %d, want %d", got, want)
	}
	observation := observations[0]
	if got, want := observation.ToolName, "missing_tool"; got != want {
		t.Fatalf("tool name = %q, want %q", got, want)
	}
	for _, want := range []string{"tool error:", "unknown tool: missing_tool"} {
		if !strings.Contains(observation.Result, want) {
			t.Fatalf("observation result = %q, want substring %q", observation.Result, want)
		}
	}
	if !strings.Contains(stdout.String(), "missing_tool -> tool error:") {
		t.Fatalf("stdout = %q, want visible tool error observation", stdout.String())
	}
}
