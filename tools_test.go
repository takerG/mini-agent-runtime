package main

import (
	"math"
	"testing"
	"time"
)

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
