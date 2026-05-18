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
	tracing "mini-agent-runtime/internal/trace"
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

type temporaryToolRetryPolicy struct{}

// Retryable 判断工具错误是否属于测试中的临时错误。
func (temporaryToolRetryPolicy) Retryable(err error) bool {
	return strings.Contains(err.Error(), "temporary")
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
				Retryable:   temporaryToolRetryPolicy{},
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

// TestRunChatLoopAsksHumanApprovalForHighRiskTool 验证 CLI 遇到高风险工具时会先请求人工确认。
func TestRunChatLoopAsksHumanApprovalForHighRiskTool(t *testing.T) {
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
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"dangerous_operation","arguments":{"action":"删除生产数据"}}}]},"done":true}` + "\n",
					)),
				}, nil
			}
			if len(requests) == 2 {
				toolMessage := requests[1].Messages[2]
				if !strings.Contains(toolMessage.Content, "【高危操作】") {
					t.Fatalf("tool message content = %q, want high-risk marker", toolMessage.Content)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"已完成模拟高危操作"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			}
			t.Fatalf("unexpected request count: %d", len(requests))
			return nil, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	traceSink := &recordingToolPolicyTraceSink{}
	err := RunChatLoopWithOptions(ChatLoopOptions{
		Endpoint:    "http://localhost:11434/api/chat",
		Model:       "llama3.2",
		Think:       true,
		Client:      client,
		InitialArgs: []string{"执行一个高危操作"},
		Stdin:       strings.NewReader("yes\n/exit\n"),
		Stdout:      &stdout,
		Stderr:      &stderr,
		Trace:       tracing.NewTraceHooks(traceSink),
	})
	if err != nil {
		t.Fatalf("RunChatLoopWithOptions returned error: %v", err)
	}
	if got, want := len(requests), 2; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	if !strings.Contains(stderr.String(), "[approval] 高风险工具调用: dangerous_operation") {
		t.Fatalf("stderr = %q, want approval prompt", stderr.String())
	}
	if got, want := stdout.String(), "已完成模拟高危操作\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if !traceSink.Has(tracing.TraceApprovalRequested) {
		t.Fatalf("trace events = %v, want approval requested", traceSink.names())
	}
	if !traceSink.Has(tracing.TraceApprovalDecided) {
		t.Fatalf("trace events = %v, want approval decided", traceSink.names())
	}
}

// TestRunChatLoopReturnsDeniedHumanApprovalToModel 验证用户拒绝高风险工具后错误会作为 observation 返回模型。
func TestRunChatLoopReturnsDeniedHumanApprovalToModel(t *testing.T) {
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
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"dangerous_operation","arguments":{"action":"删除生产数据"}}}]},"done":true}` + "\n",
					)),
				}, nil
			}
			if len(requests) == 2 {
				toolMessage := requests[1].Messages[2]
				if strings.Contains(toolMessage.Content, "【高危操作】") {
					t.Fatalf("tool message content = %q, must not contain executed marker", toolMessage.Content)
				}
				if !strings.Contains(toolMessage.Content, "human rejected high-risk tool") {
					t.Fatalf("tool message content = %q, want rejection detail", toolMessage.Content)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"已取消高危操作"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			}
			t.Fatalf("unexpected request count: %d", len(requests))
			return nil, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoopWithOptions(ChatLoopOptions{
		Endpoint:    "http://localhost:11434/api/chat",
		Model:       "llama3.2",
		Think:       true,
		Client:      client,
		InitialArgs: []string{"执行一个高危操作"},
		Stdin:       strings.NewReader("no\n/exit\n"),
		Stdout:      &stdout,
		Stderr:      &stderr,
	})
	if err != nil {
		t.Fatalf("RunChatLoopWithOptions returned error: %v", err)
	}
	if !strings.Contains(stderr.String(), "[approval] 已拒绝，本次工具不会执行。") {
		t.Fatalf("stderr = %q, want rejection prompt", stderr.String())
	}
	if got, want := stdout.String(), "已取消高危操作\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

type recordingToolPolicyTraceSink struct {
	events []tracing.TraceEvent
}

// Emit 记录工具策略测试中的 trace 事件。
func (s *recordingToolPolicyTraceSink) Emit(event tracing.TraceEvent) {
	s.events = append(s.events, event)
}

// Has 判断测试 trace 中是否包含指定事件名。
func (s *recordingToolPolicyTraceSink) Has(name tracing.TraceEventName) bool {
	for _, event := range s.events {
		if event.Name == name {
			return true
		}
	}
	return false
}

// names 返回测试 trace 中所有事件名，方便失败信息展示。
func (s *recordingToolPolicyTraceSink) names() []tracing.TraceEventName {
	names := make([]tracing.TraceEventName, 0, len(s.events))
	for _, event := range s.events {
		names = append(names, event.Name)
	}
	return names
}
