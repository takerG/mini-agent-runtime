package prompts

import (
	"strings"
	"testing"
)

// TestStrictPlannerSystemPromptUsesDynamicToolsField 验证 strict planner prompt 只引用模型请求里的 tools 字段，不写死具体工具。
func TestStrictPlannerSystemPromptUsesDynamicToolsField(t *testing.T) {
	prompt := StrictPlannerSystemPrompt()
	for _, want := range []string{
		"strict planner",
		"只能使用 tools 中声明的工具",
		"function.name",
		"JSON.parse",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, want substring %q", prompt, want)
		}
	}
	for _, forbidden := range []string{"available_tools", "current_time", "calculator"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt = %q, must not contain %q", prompt, forbidden)
		}
	}
}

// TestStrictSummarySystemPromptIncludesRuntimeContext 验证 strict responder prompt 会把 runtime context 拼入系统提示词。
func TestStrictSummarySystemPromptIncludesRuntimeContext(t *testing.T) {
	prompt := StrictSummarySystemPrompt(`{"observations":[{"tool_name":"calculator","result":"437"}]}`)
	for _, want := range []string{
		"responder",
		"runtime_context",
		"calculator",
		"437",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, want substring %q", prompt, want)
		}
	}
}

// TestExecutorSystemPromptIncludesPlanJSON 验证 executor prompt 只负责承载 plan，不耦合具体工具实现。
func TestExecutorSystemPromptIncludesPlanJSON(t *testing.T) {
	prompt := ExecutorSystemPrompt(`{"goal":"answer","steps":[{"task":"calculate"}]}`)
	for _, want := range []string{
		"executor",
		"Execution plan:",
		"calculate",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, want substring %q", prompt, want)
		}
	}
}

// TestPlannerSystemPromptUsesProvidedToolHints 验证 hybrid planner prompt 的工具提示来自调用方传入，而不是写死内置工具。
func TestPlannerSystemPromptUsesProvidedToolHints(t *testing.T) {
	prompt := PlannerSystemPrompt([]string{"mock_metric_query"})
	if !strings.Contains(prompt, "mock_metric_query") {
		t.Fatalf("prompt = %q, want dynamic tool hint", prompt)
	}
	if strings.Contains(prompt, "current_time") || strings.Contains(prompt, "calculator") {
		t.Fatalf("prompt = %q, must not hard-code built-in tools", prompt)
	}
}
