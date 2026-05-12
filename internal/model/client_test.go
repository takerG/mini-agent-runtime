package model

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"mini-agent-runtime/internal/ollama"
	tracing "mini-agent-runtime/internal/trace"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip 让测试可以用函数模拟 http.RoundTripper。
func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

// TestClientChatStreamsResponseAndEmitsFullTrace 验证模型客户端会流式输出并记录完整 trace。
func TestClientChatStreamsResponseAndEmitsFullTrace(t *testing.T) {
	var requestBody ollama.ChatRequest
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if err := json.NewDecoder(req.Body).Decode(&requestBody); err != nil {
				t.Fatalf("decode upstream request: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"hi"}}` + "\n" +
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"calculator","arguments":{"a":2,"b":3,"op":"+"}}}]}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}
	sink := &recordingTraceSink{}
	var stdout strings.Builder
	think := true

	client := NewClient(Options{
		Endpoint: "http://localhost:11434/api/chat",
		Model:    "qwen3:4b",
		Think:    think,
		HTTP:     httpClient,
		Trace:    tracing.NewTraceHooks(sink),
	})

	result, err := client.Chat(t.Context(), ChatOptions{
		Phase:     "chat",
		ToolRound: 1,
		Messages: []ollama.Message{
			{Role: "user", Content: "hello"},
		},
		Tools: []ollama.ToolDefinition{
			{
				Type: "function",
				Function: ollama.ToolDescription{
					Name:        "calculator",
					Description: "calculate",
				},
			},
		},
		Stream: ollama.StreamOptions{Writer: &stdout},
	})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	if got, want := stdout.String(), "hi"; got != want {
		t.Fatalf("streamed output = %q, want %q", got, want)
	}
	if got, want := result.Content, "hi"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	if got, want := len(result.ToolCalls), 1; got != want {
		t.Fatalf("tool calls = %d, want %d", got, want)
	}
	if got, want := requestBody.Messages[0].Content, "hello"; got != want {
		t.Fatalf("request user content = %q, want %q", got, want)
	}

	requestTrace := traceData[tracing.ModelRequestTrace](t, sink.events, tracing.TraceModelRequest)
	if got, want := requestTrace.Phase, "chat"; got != want {
		t.Fatalf("request phase = %q, want %q", got, want)
	}
	if got, want := requestTrace.Request.Messages[0].Content, "hello"; got != want {
		t.Fatalf("trace request content = %q, want %q", got, want)
	}
	responseTrace := traceData[tracing.ModelResponseTrace](t, sink.events, tracing.TraceModelResponse)
	if got, want := responseTrace.Content, "hi"; got != want {
		t.Fatalf("trace response content = %q, want %q", got, want)
	}
	if got, want := responseTrace.ToolCalls[0].Function.Name, "calculator"; got != want {
		t.Fatalf("trace tool call = %q, want %q", got, want)
	}
}

// traceData 从测试事件列表中读取指定类型的 trace 数据。
func traceData[T any](t *testing.T, events []tracing.TraceEvent, name tracing.TraceEventName) T {
	t.Helper()
	var zero T
	for _, event := range events {
		if event.Name != name {
			continue
		}
		data, ok := event.Data.(T)
		if !ok {
			t.Fatalf("trace %s data type = %T", name, event.Data)
		}
		return data
	}
	t.Fatalf("missing trace event %s", name)
	return zero
}

type recordingTraceSink struct {
	events []tracing.TraceEvent
}

// Emit 把 trace 事件追加到测试 sink 中，便于后续断言。
func (s *recordingTraceSink) Emit(event tracing.TraceEvent) {
	s.events = append(s.events, event)
}
