package planner

import "testing"

// TestParseExecutablePlanParsesToolCallSteps 验证 strict planner JSON 能解析成工具调用步骤。
func TestParseExecutablePlanParsesToolCallSteps(t *testing.T) {
	plan, err := ParseExecutablePlan(`{
		"goal": "make a record",
		"steps": [
			{"type": "tool_call", "tool_name": "calculator", "arguments": {"a": 23, "b": 19, "op": "*"}},
			{"type": "tool_call", "tool_name": "current_time", "arguments": {}}
		]
	}`)
	if err != nil {
		t.Fatalf("ParseExecutablePlan returned error: %v", err)
	}

	if got, want := plan.Goal, "make a record"; got != want {
		t.Fatalf("goal = %q, want %q", got, want)
	}
	if got, want := len(plan.Steps), 2; got != want {
		t.Fatalf("step count = %d, want %d", got, want)
	}
	if got, want := plan.Steps[0].Type, "tool_call"; got != want {
		t.Fatalf("first step type = %q, want %q", got, want)
	}
	if got, want := plan.Steps[0].ToolName, "calculator"; got != want {
		t.Fatalf("first tool name = %q, want %q", got, want)
	}
	if got, want := plan.Steps[0].Arguments["op"], "*"; got != want {
		t.Fatalf("first op = %#v, want %#v", got, want)
	}
}

// TestParseExecutablePlanRejectsMissingToolName 验证缺少工具名称的可执行计划会被拒绝。
func TestParseExecutablePlanRejectsMissingToolName(t *testing.T) {
	_, err := ParseExecutablePlan(`{"goal":"bad","steps":[{"type":"tool_call","arguments":{}}]}`)
	if err == nil {
		t.Fatal("ParseExecutablePlan returned nil error, want missing tool name error")
	}
}
