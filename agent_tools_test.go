package main

import (
	"strings"
	"testing"
	"time"
)

type echoTool struct{}

func (echoTool) Definition() toolDefinition {
	return toolDefinition{
		Type: "function",
		Function: toolDescription{
			Name:        "echo",
			Description: "return the provided text",
			Parameters:  map[string]any{"type": "object"},
		},
	}
}

func (echoTool) Execute(args map[string]any) (string, error) {
	return "echo:" + args["text"].(string), nil
}

func TestToolRegistryRegistersToolImplementations(t *testing.T) {
	registry := NewToolRegistry()
	err := registry.Register(echoTool{})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	definitions := registry.Definitions()
	if got, want := len(definitions), 1; got != want {
		t.Fatalf("definition count = %d, want %d", got, want)
	}
	if got, want := definitions[0].Function.Name, "echo"; got != want {
		t.Fatalf("definition name = %q, want %q", got, want)
	}

	result, err := registry.Execute(toolCall{
		Function: toolFunctionCall{
			Name: "echo",
			Arguments: map[string]any{
				"text": "hello",
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if want := "echo:hello"; result != want {
		t.Fatalf("Execute result = %q, want %q", result, want)
	}
}

func TestToolRegistryRejectsDuplicateToolName(t *testing.T) {
	registry := NewToolRegistry()
	if err := registry.Register(echoTool{}); err != nil {
		t.Fatalf("first Register returned error: %v", err)
	}
	if err := registry.Register(echoTool{}); err == nil {
		t.Fatal("second Register returned nil error, want duplicate error")
	}
}

func TestBuiltInToolsImplementToolInterface(t *testing.T) {
	var tools []Tool = []Tool{
		CurrentTimeAgentTool{Now: func() time.Time {
			return time.Date(2026, 5, 2, 18, 30, 0, 0, time.FixedZone("CST", 8*60*60))
		}},
		CalculatorAgentTool{},
	}

	if got, want := tools[0].Definition().Function.Name, "current_time"; got != want {
		t.Fatalf("current time tool name = %q, want %q", got, want)
	}
	if got, want := tools[1].Definition().Function.Name, "calculator"; got != want {
		t.Fatalf("calculator tool name = %q, want %q", got, want)
	}
}

func TestDefaultToolRegistryIncludesBuiltInTools(t *testing.T) {
	registry := DefaultToolRegistry(func() time.Time {
		return time.Date(2026, 5, 2, 18, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	})

	definitions := registry.Definitions()
	if got, want := len(definitions), 2; got != want {
		t.Fatalf("definition count = %d, want %d", got, want)
	}
	if got, want := definitions[0].Function.Name, "current_time"; got != want {
		t.Fatalf("first tool name = %q, want %q", got, want)
	}
	if got, want := definitions[1].Function.Name, "calculator"; got != want {
		t.Fatalf("second tool name = %q, want %q", got, want)
	}
}

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
