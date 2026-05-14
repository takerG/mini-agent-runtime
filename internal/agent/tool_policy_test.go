package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"mini-agent-runtime/internal/ollama"
	"mini-agent-runtime/internal/tools"
)

type flakyTool struct {
	attempts int
}

// Name 返回测试用 flaky 工具名称。
func (t *flakyTool) Name() string {
	return "flaky_tool"
}

// Description 返回测试用 flaky 工具说明。
func (t *flakyTool) Description() string {
	return "fail once then succeed"
}

// Definition 返回测试用 flaky 工具定义。
func (t *flakyTool) Definition() ollama.ToolDefinition {
	return ollama.ToolDefinition{
		Type: "function",
		Function: ollama.ToolDescription{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  map[string]any{"type": "object"},
		},
	}
}

// Execute 第一次失败、第二次成功，用于验证 agent 注入的工具策略。
func (t *flakyTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	t.attempts++
	if t.attempts == 1 {
		return "", errors.New("temporary tool failure")
	}
	return "tool-ok", nil
}

// TestRunChatLoopUsesInjectedToolExecutionPolicy 验证 CLI 运行链路会把注入的工具执行策略传给工具注册表。
func TestRunChatLoopUsesInjectedToolExecutionPolicy(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)
			if len(requests) == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"flaky_tool","arguments":{}}}]},"done":true}` + "\n",
					)),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"done"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	registry := tools.NewToolRegistry()
	tool := &flakyTool{}
	registry.Register(tool)
	err := RunChatLoopWithOptions(ChatLoopOptions{
		Endpoint:    "http://localhost:11434/api/chat",
		Model:       "llama3.2",
		Think:       true,
		Client:      client,
		InitialArgs: []string{"run flaky tool"},
		Stdin:       strings.NewReader("/exit\n"),
		Stdout:      io.Discard,
		Stderr:      io.Discard,
		Dependencies: ChatLoopDependencies{
			Tools: registry,
			ToolPolicy: tools.ExecutionPolicy{
				MaxAttempts: 2,
				Retryable: func(err error) bool {
					return strings.Contains(err.Error(), "temporary")
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("RunChatLoopWithOptions returned error: %v", err)
	}
	if got, want := tool.attempts, 2; got != want {
		t.Fatalf("tool attempts = %d, want %d", got, want)
	}
}
