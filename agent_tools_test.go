package main

import (
	"strings"
	"testing"
	"time"
)

func TestExecuteToolCallRunsCurrentTimeTool(t *testing.T) {
	call := toolCall{
		Function: toolFunctionCall{
			Name:      "current_time",
			Arguments: map[string]any{},
		},
	}
	now := func() time.Time {
		return time.Date(2026, 4, 30, 18, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	}

	got, err := ExecuteToolCall(call, now)
	if err != nil {
		t.Fatalf("ExecuteToolCall returned error: %v", err)
	}
	if want := "2026-04-30 18:30:00 CST"; got != want {
		t.Fatalf("tool result = %q, want %q", got, want)
	}
}

func TestExecuteToolCallRunsCalculatorTool(t *testing.T) {
	call := toolCall{
		Function: toolFunctionCall{
			Name: "calculator",
			Arguments: map[string]any{
				"op": "*",
				"a":  6.0,
				"b":  7.0,
			},
		},
	}

	got, err := ExecuteToolCall(call, time.Now)
	if err != nil {
		t.Fatalf("ExecuteToolCall returned error: %v", err)
	}
	if want := "42"; got != want {
		t.Fatalf("tool result = %q, want %q", got, want)
	}
}

func TestExecuteToolCallReturnsErrorForBadCalculatorArguments(t *testing.T) {
	call := toolCall{
		Function: toolFunctionCall{
			Name: "calculator",
			Arguments: map[string]any{
				"op": "/",
				"a":  1.0,
				"b":  0.0,
			},
		},
	}

	_, err := ExecuteToolCall(call, time.Now)
	if err == nil {
		t.Fatal("ExecuteToolCall returned nil error, want division error")
	}
	if !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("error = %q, want division by zero", err.Error())
	}
}
