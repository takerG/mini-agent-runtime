package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"mini-agent-runtime/internal/ollama"
)

type retryTool struct {
	attempts int
}

// Name 返回重试测试工具的名称。
func (t *retryTool) Name() string {
	return "retry_tool"
}

// Description 返回重试测试工具的说明。
func (t *retryTool) Description() string {
	return "fail once then succeed"
}

// Definition 返回重试测试工具的模型可见定义。
func (t *retryTool) Definition() ollama.ToolDefinition {
	return ollama.ToolDefinition{Type: "function", Function: ollama.ToolDescription{Name: t.Name(), Description: t.Description()}}
}

// Execute 第一次调用失败，第二次调用成功，用于验证 retry policy。
func (t *retryTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	t.attempts++
	if t.attempts == 1 {
		return "", errors.New("temporary failure")
	}
	return "ok", nil
}

type slowTool struct{}

// Name 返回超时测试工具的名称。
func (t *slowTool) Name() string {
	return "slow_tool"
}

// Description 返回超时测试工具的说明。
func (t *slowTool) Description() string {
	return "wait until context is cancelled"
}

// Definition 返回超时测试工具的模型可见定义。
func (t *slowTool) Definition() ollama.ToolDefinition {
	return ollama.ToolDefinition{Type: "function", Function: ollama.ToolDescription{Name: t.Name(), Description: t.Description()}}
}

// Execute 阻塞直到 context 被取消，用于验证 timeout policy。
func (t *slowTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

// TestToolRegistryExecuteWithPolicyRetriesRetryableErrors 验证工具策略可以控制可重试错误的重试次数。
func TestToolRegistryExecuteWithPolicyRetriesRetryableErrors(t *testing.T) {
	registry := NewToolRegistry()
	tool := &retryTool{}
	registry.Register(tool)

	result, err := registry.ExecuteWithPolicy(context.Background(), ollama.ToolCall{
		Function: ollama.ToolFunctionCall{Name: "retry_tool"},
	}, ExecutionPolicy{
		MaxAttempts: 2,
		Retryable: func(err error) bool {
			return strings.Contains(err.Error(), "temporary")
		},
	})
	if err != nil {
		t.Fatalf("ExecuteWithPolicy returned error: %v", err)
	}
	if got, want := result, "ok"; got != want {
		t.Fatalf("result = %q, want %q", got, want)
	}
	if got, want := tool.attempts, 2; got != want {
		t.Fatalf("attempts = %d, want %d", got, want)
	}
}

// TestToolRegistryExecuteWithPolicyTimesOutSlowTools 验证工具策略可以为单次工具执行设置超时。
func TestToolRegistryExecuteWithPolicyTimesOutSlowTools(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(&slowTool{})

	_, err := registry.ExecuteWithPolicy(context.Background(), ollama.ToolCall{
		Function: ollama.ToolFunctionCall{Name: "slow_tool"},
	}, ExecutionPolicy{Timeout: time.Millisecond})
	if err == nil {
		t.Fatal("ExecuteWithPolicy returned nil error, want timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %q, want timeout message", err.Error())
	}
}

// TestToolRegistryExecuteWithPolicyDeniesDisallowedTools 验证工具策略可以在执行前拒绝工具调用。
func TestToolRegistryExecuteWithPolicyDeniesDisallowedTools(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(&retryTool{})

	_, err := registry.ExecuteWithPolicy(context.Background(), ollama.ToolCall{
		Function: ollama.ToolFunctionCall{Name: "retry_tool"},
	}, ExecutionPolicy{
		Allow: func(call ollama.ToolCall) error {
			return errors.New("approval required")
		},
	})
	if err == nil {
		t.Fatal("ExecuteWithPolicy returned nil error, want policy error")
	}
	if !strings.Contains(err.Error(), "approval required") {
		t.Fatalf("error = %q, want policy denial", err.Error())
	}
}
