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

// TestRunChatLoopWithTraceLogsAgentToolFlow 验证 trace hooks 能记录普通 agent 工具调用流程。
func TestRunChatLoopWithTraceLogsAgentToolFlow(t *testing.T) {
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
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"calculator","arguments":{"op":"+","a":2,"b":3}}}]},"done":true}` + "\n",
					)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"2+3=5"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	sink := &recordingTraceSink{}
	err := RunChatLoopWithTrace(
		"http://localhost:11434/api/chat",
		"llama3.2",
		true,
		client,
		[]string{"2+3是多少？"},
		strings.NewReader("/exit\n"),
		&stdout,
		&stderr,
		tracing.NewTraceHooks(sink),
	)
	if err != nil {
		t.Fatalf("RunChatLoopWithTrace returned error: %v", err)
	}

	gotNames := make([]tracing.TraceEventName, 0, len(sink.events))
	for _, event := range sink.events {
		gotNames = append(gotNames, event.Name)
	}
	for _, want := range []tracing.TraceEventName{
		tracing.TraceChatLoopStart,
		tracing.TraceTurnInput,
		tracing.TraceModelRequest,
		tracing.TraceModelResponse,
		tracing.TraceToolCall,
		tracing.TraceToolResult,
		tracing.TraceModelRequest,
		tracing.TraceModelResponse,
		tracing.TraceFinalAnswer,
	} {
		if !traceNamesContain(gotNames, want) {
			t.Fatalf("trace events missing %q in %#v", want, gotNames)
		}
	}

	requestTrace, ok := firstTraceData[tracing.ModelRequestTrace](sink.events, tracing.TraceModelRequest)
	if !ok {
		t.Fatal("missing first model request trace data")
	}
	if got := requestTrace.Request.Messages[0].Content; !strings.Contains(got, "2+3") {
		t.Fatalf("request trace user content = %q, want to contain 2+3", got)
	}
	responseTrace, ok := firstTraceData[tracing.ModelResponseTrace](sink.events, tracing.TraceModelResponse)
	if !ok {
		t.Fatal("missing first model response trace data")
	}
	if got, want := len(responseTrace.ToolCalls), 1; got != want {
		t.Fatalf("response trace tool call count = %d, want %d", got, want)
	}
	if got, want := responseTrace.ToolCalls[0].Function.Name, "calculator"; got != want {
		t.Fatalf("response trace tool call name = %q, want %q", got, want)
	}
}

// TestRunChatLoopWithTraceLogsPlannerExecutorFlow 验证 trace hooks 能记录 planner/executor 流程。
func TestRunChatLoopWithTraceLogsPlannerExecutorFlow(t *testing.T) {
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
						`{"message":{"content":"{\"goal\":\"answer\",\"steps\":[{\"task\":\"answer directly\"}]}"}}` + "\n" +
							`{"done":true}` + "\n",
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

	var stdout strings.Builder
	var stderr strings.Builder
	sink := &recordingTraceSink{}
	err := RunChatLoopWithOptions(ChatLoopOptions{
		Endpoint:    "http://localhost:11434/api/chat",
		Model:       "llama3.2",
		Think:       true,
		Client:      client,
		InitialArgs: []string{"answer"},
		Stdin:       strings.NewReader("/exit\n"),
		Stdout:      &stdout,
		Stderr:      &stderr,
		Trace:       tracing.NewTraceHooks(sink),
		Mode:        ModePlan,
	})
	if err != nil {
		t.Fatalf("RunChatLoopWithOptions returned error: %v", err)
	}

	gotNames := make([]tracing.TraceEventName, 0, len(sink.events))
	for _, event := range sink.events {
		gotNames = append(gotNames, event.Name)
	}
	for _, want := range []tracing.TraceEventName{
		tracing.TracePlannerRequest,
		tracing.TracePlannerResponse,
		tracing.TraceExecutorStart,
		tracing.TraceExecutorStep,
		tracing.TraceExecutorFinish,
	} {
		if !traceNamesContain(gotNames, want) {
			t.Fatalf("trace events missing %q in %#v", want, gotNames)
		}
	}
}

// traceNamesContain 判断 trace 名称列表中是否包含目标事件名。
func traceNamesContain(names []tracing.TraceEventName, want tracing.TraceEventName) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

// firstTraceData 返回指定 trace 事件名对应的第一份结构化数据。
func firstTraceData[T any](events []tracing.TraceEvent, name tracing.TraceEventName) (T, bool) {
	var zero T
	for _, event := range events {
		if event.Name != name {
			continue
		}
		data, ok := event.Data.(T)
		if !ok {
			return zero, false
		}
		return data, true
	}
	return zero, false
}

type recordingTraceSink struct {
	events []tracing.TraceEvent
}

// Emit 把 trace 事件追加到测试 sink 中，便于断言事件顺序和内容。
func (s *recordingTraceSink) Emit(event tracing.TraceEvent) {
	s.events = append(s.events, event)
}
