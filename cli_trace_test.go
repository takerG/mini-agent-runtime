package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRunChatLoopWithTraceLogsAgentToolFlow(t *testing.T) {
	var requests []chatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body chatRequest
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
		NewTraceHooks(sink),
	)
	if err != nil {
		t.Fatalf("RunChatLoopWithTrace returned error: %v", err)
	}

	gotNames := make([]TraceEventName, 0, len(sink.events))
	for _, event := range sink.events {
		gotNames = append(gotNames, event.Name)
	}
	for _, want := range []TraceEventName{
		TraceChatLoopStart,
		TraceTurnInput,
		TraceModelRequest,
		TraceModelResponse,
		TraceToolCall,
		TraceToolResult,
		TraceModelRequest,
		TraceModelResponse,
		TraceFinalAnswer,
	} {
		if !traceNamesContain(gotNames, want) {
			t.Fatalf("trace events missing %q in %#v", want, gotNames)
		}
	}
}

func traceNamesContain(names []TraceEventName, want TraceEventName) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
