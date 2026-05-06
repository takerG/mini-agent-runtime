package tools

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"mini-agent-runtime/internal/ollama"
)

func TestCalculatorToolImplementsToolInterface(t *testing.T) {
	var tool Tool = NewCalculatorTool()

	if got, want := tool.Name(), "calculator"; got != want {
		t.Fatalf("tool name = %q, want %q", got, want)
	}
	if tool.Description() == "" {
		t.Fatal("tool description is empty")
	}
	if got, want := tool.Definition().Function.Name, "calculator"; got != want {
		t.Fatalf("definition name = %q, want %q", got, want)
	}
}

func TestCalculatorToolSupportsFourBasicOperations(t *testing.T) {
	tests := []struct {
		name string
		op   string
		a    float64
		b    float64
		want float64
	}{
		{name: "add", op: "+", a: 2, b: 3, want: 5},
		{name: "subtract", op: "-", a: 9, b: 4, want: 5},
		{name: "multiply", op: "*", a: 6, b: 7, want: 42},
		{name: "divide", op: "/", a: 8, b: 2, want: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculatorTool(tt.op, tt.a, tt.b)
			if err != nil {
				t.Fatalf("CalculatorTool returned error: %v", err)
			}
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("CalculatorTool(%q, %v, %v) = %v, want %v", tt.op, tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCalculatorToolRejectsDivisionByZero(t *testing.T) {
	if _, err := CalculatorTool("/", 1, 0); err == nil {
		t.Fatal("CalculatorTool returned nil error, want division by zero error")
	}
}

func TestCalculatorToolRejectsUnknownOperation(t *testing.T) {
	if _, err := CalculatorTool("%", 1, 2); err == nil {
		t.Fatal("CalculatorTool returned nil error, want unknown operation error")
	}
}

func TestDefaultToolRegistryRunsCalculatorTool(t *testing.T) {
	registry := NewDefaultToolRegistry(time.Now)

	got, err := registry.Execute(context.Background(), ollama.ToolCall{
		Function: ollama.ToolFunctionCall{
			Name: "calculator",
			Arguments: map[string]any{
				"op": "*",
				"a":  6.0,
				"b":  7.0,
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if want := "42"; got != want {
		t.Fatalf("tool result = %q, want %q", got, want)
	}
}

func TestDefaultToolRegistryReturnsCalculatorErrors(t *testing.T) {
	registry := NewDefaultToolRegistry(time.Now)

	_, err := registry.Execute(context.Background(), ollama.ToolCall{
		Function: ollama.ToolFunctionCall{
			Name: "calculator",
			Arguments: map[string]any{
				"op": "/",
				"a":  1.0,
				"b":  0.0,
			},
		},
	})
	if err == nil {
		t.Fatal("Execute returned nil error, want division error")
	}
	if !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("error = %q, want division by zero", err.Error())
	}
}
