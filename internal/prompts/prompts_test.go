package prompts

import (
	"strings"
	"testing"
)

// TestStrictPlannerSystemPromptUsesDynamicToolsField 验证 strict planner prompt 只引用模型请求里的 tools 字段，不写死具体工具。
func TestStrictPlannerSystemPromptUsesDynamicToolsField(t *testing.T) {
	prompt := StrictPlannerSystemPrompt()
	for _, want := range []string{
		"严格规划器",
		"只能使用 tools 中声明的工具",
		"function.name",
		"当前实现不会解析步骤引用",
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
	if strings.Contains(prompt, "$step_id.result") {
		t.Fatalf("prompt = %q, must not require unsupported step result references", prompt)
	}
}

// TestStrictSummarySystemPromptIncludesRuntimeContext 验证 strict responder prompt 会把 runtime context 拼入系统提示词。
func TestStrictSummarySystemPromptIncludesRuntimeContext(t *testing.T) {
	prompt := StrictSummarySystemPrompt(`{"observations":[{"tool_name":"calculator","result":"437"}]}`)
	for _, want := range []string{
		"最终回答器",
		"runtime_context",
		"unsupported request",
		"工具错误",
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
		"执行器",
		"执行计划：",
		"不要编造工具结果",
		"工具错误",
		"calculate",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, want substring %q", prompt, want)
		}
	}
	for _, forbidden := range []string{
		"You are the executor",
		"Follow the plan",
		"Execution plan:",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt = %q, must not contain English instruction %q", prompt, forbidden)
		}
	}
}

// TestPlannerSystemPromptUsesProvidedToolHints 验证 hybrid planner prompt 的工具提示来自调用方传入，而不是写死内置工具。
func TestPlannerSystemPromptUsesProvidedToolHints(t *testing.T) {
	prompt := PlannerSystemPrompt([]string{"mock_metric_query"})
	if !strings.Contains(prompt, "mock_metric_query") {
		t.Fatalf("prompt = %q, want dynamic tool hint", prompt)
	}
	for _, want := range []string{
		"规划器",
		"可用工具提示",
		"tool_hint 必须来自可用工具提示清单",
		"不要把工具执行结果写入计划",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, want substring %q", prompt, want)
		}
	}
	if strings.Contains(prompt, "current_time") || strings.Contains(prompt, "calculator") {
		t.Fatalf("prompt = %q, must not hard-code built-in tools", prompt)
	}
	for _, forbidden := range []string{
		"You are the planner",
		"Create a concise execution plan",
		"Available tool hints",
		"No tool hints",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt = %q, must not contain English instruction %q", prompt, forbidden)
		}
	}
}

// TestPlannerSystemPromptUsesChineseEmptyToolHint 验证没有工具提示时也使用中文说明。
func TestPlannerSystemPromptUsesChineseEmptyToolHint(t *testing.T) {
	prompt := PlannerSystemPrompt(nil)
	if !strings.Contains(prompt, "工具提示清单为空；不要填写 tool_hint。") {
		t.Fatalf("prompt = %q, want Chinese empty tool hint", prompt)
	}
}

// TestMemorySummarySystemPromptAvoidsSensitiveTransientData 验证 memory 摘要 prompt 会约束长期记忆的内容边界。
func TestMemorySummarySystemPromptAvoidsSensitiveTransientData(t *testing.T) {
	prompt := MemorySummarySystemPrompt()
	for _, want := range []string{
		"不要记录密码、token、密钥",
		"没有新增长期记忆时",
		"不要输出 Markdown",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, want substring %q", prompt, want)
		}
	}
}
