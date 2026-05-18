package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"mini-agent-runtime/internal/ollama"
)

type echoTool struct {
	seenContext context.Context
}

type registryContextKey string

const requestIDContextKey registryContextKey = "request_id"

// Name 返回测试 echo 工具的注册名称。
func (t *echoTool) Name() string {
	return "echo"
}

// Description 返回测试 echo 工具的说明文本。
func (t *echoTool) Description() string {
	return "return the provided text"
}

// Definition 返回测试 echo 工具的模型可见定义。
func (t *echoTool) Definition() ollama.ToolDefinition {
	return ollama.ToolDefinition{
		Type: "function",
		Function: ollama.ToolDescription{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  map[string]any{"type": "object"},
		},
	}
}

// Execute 返回测试 echo 工具参数中的 text 字段。
func (t *echoTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	t.seenContext = ctx
	return "echo:" + args["text"].(string), nil
}

// TestToolRegistryRegistersToolImplementations 验证注册表能注册、暴露并执行工具实现。
func TestToolRegistryRegistersToolImplementations(t *testing.T) {
	registry := NewToolRegistry()
	tool := &echoTool{}
	registry.Register(tool)

	definitions := registry.Definitions()
	if got, want := len(definitions), 1; got != want {
		t.Fatalf("definition count = %d, want %d", got, want)
	}
	if got, want := definitions[0].Function.Name, "echo"; got != want {
		t.Fatalf("definition name = %q, want %q", got, want)
	}

	ctx := context.WithValue(context.Background(), requestIDContextKey, "test-request")
	result, err := registry.Execute(ctx, ollama.ToolCall{
		Function: ollama.ToolFunctionCall{
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
	if tool.seenContext != ctx {
		t.Fatal("Execute did not pass context to tool")
	}
}

// TestToolRegistryReturnsErrorForUnknownTool 验证未知工具调用会返回稳定错误。
func TestToolRegistryReturnsErrorForUnknownTool(t *testing.T) {
	registry := NewToolRegistry()
	_, err := registry.Execute(context.Background(), ollama.ToolCall{
		Function: ollama.ToolFunctionCall{Name: "missing_tool"},
	})
	if err == nil {
		t.Fatal("Execute returned nil error, want unknown tool error")
	}
	if !strings.Contains(err.Error(), "unknown tool: missing_tool") {
		t.Fatalf("error = %q, want unknown tool message", err.Error())
	}
}

// TestNewDefaultToolRegistryIncludesBuiltInTools 验证默认注册表包含内置工具集合。
func TestNewDefaultToolRegistryIncludesBuiltInTools(t *testing.T) {
	registry := NewDefaultToolRegistry(func() time.Time {
		return time.Date(2026, 5, 2, 18, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	})

	definitions := registry.Definitions()
	if got, want := len(definitions), 3; got != want {
		t.Fatalf("definition count = %d, want %d", got, want)
	}

	names := map[string]bool{}
	for _, definition := range definitions {
		names[definition.Function.Name] = true
	}
	for _, want := range []string{"current_time", "calculator", "dangerous_operation"} {
		if !names[want] {
			t.Fatalf("registered tool names = %v, want %q", names, want)
		}
	}
}
