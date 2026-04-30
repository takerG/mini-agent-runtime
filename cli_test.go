package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRunChatLoopSendsConversationHistoryAcrossTurns(t *testing.T) {
	var requests []chatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body chatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			answer := "answer one"
			if len(requests) == 2 {
				answer = "answer two"
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"` + answer + `"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoop(
		"http://localhost:11434/api/chat",
		"llama3.2",
		client,
		nil,
		strings.NewReader("first\nsecond\n/exit\n"),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("RunChatLoop returned error: %v", err)
	}

	if got, want := len(requests), 2; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	if got, want := len(requests[0].Messages), 1; got != want {
		t.Fatalf("first request message count = %d, want %d", got, want)
	}
	if got, want := requests[0].Messages[0], (chatMessage{Role: "user", Content: "first"}); got != want {
		t.Fatalf("first request message = %#v, want %#v", got, want)
	}
	if requests[0].Think == nil || *requests[0].Think {
		t.Fatalf("first request think = %v, want false", requests[0].Think)
	}

	wantHistory := []chatMessage{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "answer one"},
		{Role: "user", Content: "second"},
	}
	if got := requests[1].Messages; len(got) != len(wantHistory) {
		t.Fatalf("second request message count = %d, want %d", len(got), len(wantHistory))
	}
	for i, want := range wantHistory {
		if requests[1].Messages[i] != want {
			t.Fatalf("second request message[%d] = %#v, want %#v", i, requests[1].Messages[i], want)
		}
	}
	if got, want := stdout.String(), "answer one\nanswer two\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunChatLoopUsesArgsAsFirstMessageThenContinuesReadingStdin(t *testing.T) {
	var requests []chatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body chatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"ok"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoop(
		"http://localhost:11434/api/chat",
		"llama3.2",
		client,
		[]string{"from", "args"},
		strings.NewReader("/exit\n"),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("RunChatLoop returned error: %v", err)
	}

	if got, want := len(requests), 1; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	if got, want := requests[0].Messages[0], (chatMessage{Role: "user", Content: "from args"}); got != want {
		t.Fatalf("first request message = %#v, want %#v", got, want)
	}
}
