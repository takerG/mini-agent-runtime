package agent

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"mini-agent-runtime/internal/ollama"
	tracing "mini-agent-runtime/internal/trace"
)

// TestRunChatLoopTraceIncludesRunLifecycleContext 验证单轮对话会输出 run/step 生命周期事件，并把上下文挂到模型请求上。
func TestRunChatLoopTraceIncludesRunLifecycleContext(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"hello"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	sink := &recordingTraceSink{}
	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoopWithOptions(ChatLoopOptions{
		Endpoint:    "http://localhost:11434/api/chat",
		Model:       "llama3.2",
		Think:       true,
		Client:      client,
		InitialArgs: []string{"hello"},
		Stdin:       strings.NewReader("/exit\n"),
		Stdout:      &stdout,
		Stderr:      &stderr,
		Trace:       tracing.NewTraceHooks(sink),
	})
	if err != nil {
		t.Fatalf("RunChatLoopWithOptions returned error: %v", err)
	}

	for _, want := range []tracing.TraceEventName{
		tracing.TraceRunStart,
		tracing.TraceStepStart,
		tracing.TraceModelRequest,
		tracing.TraceModelResponse,
		tracing.TraceStepFinish,
		tracing.TraceRunFinish,
	} {
		if !traceNamesContain(traceNames(sink.events), want) {
			t.Fatalf("trace events missing %q in %#v", want, traceNames(sink.events))
		}
	}

	modelRequest := firstTraceEvent(sink.events, tracing.TraceModelRequest)
	if modelRequest.RunID == "" {
		t.Fatal("model request run id is empty")
	}
	if modelRequest.StepID == "" {
		t.Fatal("model request step id is empty")
	}
}

// traceNames 提取 trace 事件名称列表，便于测试断言。
func traceNames(events []tracing.TraceEvent) []tracing.TraceEventName {
	names := make([]tracing.TraceEventName, 0, len(events))
	for _, event := range events {
		names = append(names, event.Name)
	}
	return names
}

// firstTraceEvent 返回指定名称对应的第一条 trace 事件。
func firstTraceEvent(events []tracing.TraceEvent, name tracing.TraceEventName) tracing.TraceEvent {
	for _, event := range events {
		if event.Name == name {
			return event
		}
	}
	return tracing.TraceEvent{}
}
