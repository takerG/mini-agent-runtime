package executor

import (
	"context"
	"strings"
	"testing"

	"mini-agent-runtime/internal/planner"
)

// TestNewExecutorDoesNotCreateRuntimeDefaults 验证普通 executor 构造函数不再重复创建 runtime 层默认依赖。
func TestNewExecutorDoesNotCreateRuntimeDefaults(t *testing.T) {
	executor := NewExecutor(Options{})

	if executor.registry != nil {
		t.Fatal("registry should be provided by runtime, not created by NewExecutor")
	}
	if executor.trace != nil {
		t.Fatal("trace hooks should be provided by runtime, not created by NewExecutor")
	}
	if executor.reporter != nil {
		t.Fatal("reporter should be provided by runtime, not created by NewExecutor")
	}
	if executor.toolPolicy.MaxAttempts != 0 {
		t.Fatalf("tool policy max attempts = %d, want zero value from caller", executor.toolPolicy.MaxAttempts)
	}
	if executor.stdout == nil {
		t.Fatal("stdout should still default to io.Discard because it is a local output safety guard")
	}
}

// TestNewStrictExecutorDoesNotCreateRuntimeDefaults 验证 strict executor 构造函数不再重复创建 runtime 层默认依赖。
func TestNewStrictExecutorDoesNotCreateRuntimeDefaults(t *testing.T) {
	executor := NewStrictExecutor(StrictExecutorOptions{})

	if executor.registry != nil {
		t.Fatal("registry should be provided by runtime, not created by NewStrictExecutor")
	}
	if executor.trace != nil {
		t.Fatal("trace hooks should be provided by runtime, not created by NewStrictExecutor")
	}
	if executor.reporter != nil {
		t.Fatal("reporter should be provided by runtime, not created by NewStrictExecutor")
	}
	if executor.toolPolicy.MaxAttempts != 0 {
		t.Fatalf("tool policy max attempts = %d, want zero value from caller", executor.toolPolicy.MaxAttempts)
	}
	if executor.stdout == nil {
		t.Fatal("stdout should still default to io.Discard because it is a local output safety guard")
	}
}

// TestExecutorReturnsErrorWhenRegistryMissing 验证普通 executor 缺少 registry 时返回明确配置错误。
func TestExecutorReturnsErrorWhenRegistryMissing(t *testing.T) {
	executor := NewExecutor(Options{})

	_, err := executor.Execute(context.Background(), "hello", planner.Plan{Goal: "answer"})
	if err == nil {
		t.Fatal("Execute returned nil error, want missing registry error")
	}
	if !strings.Contains(err.Error(), "executor requires tool registry") {
		t.Fatalf("error = %q, want missing registry message", err.Error())
	}
}

// TestStrictExecutorTurnsMissingRegistryIntoObservation 验证 strict executor 缺少 registry 时不会 panic，而是输出工具错误 observation。
func TestStrictExecutorTurnsMissingRegistryIntoObservation(t *testing.T) {
	executor := NewStrictExecutor(StrictExecutorOptions{})

	observations := executor.Execute(context.Background(), planner.ExecutablePlan{
		Goal: "call tool",
		Steps: []planner.ExecutableStep{
			{Type: planner.ExecutableStepToolCall, ToolName: "calculator", Arguments: map[string]any{}},
		},
	})
	if got, want := len(observations), 1; got != want {
		t.Fatalf("observation count = %d, want %d", got, want)
	}
	if !strings.Contains(observations[0].Result, "strict executor requires tool registry") {
		t.Fatalf("observation result = %q, want missing registry message", observations[0].Result)
	}
}
