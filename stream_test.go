package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type flushingBuffer struct {
	bytes.Buffer
	flushes int
}

func (b *flushingBuffer) Flush() {
	b.flushes++
}

func TestStreamChatContentWritesMessageContentChunks(t *testing.T) {
	input := strings.NewReader(
		`{"message":{"content":"hello"}}` + "\n" +
			`{"message":{"content":" world"}}` + "\n" +
			`{"done":true}` + "\n",
	)
	var output bytes.Buffer

	if err := StreamChatContent(input, &output); err != nil {
		t.Fatalf("StreamChatContent returned error: %v", err)
	}

	if got, want := output.String(), "hello world"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestStreamChatContentFlushesAfterEachMessageContentChunk(t *testing.T) {
	input := strings.NewReader(
		`{"message":{"content":"a"}}` + "\n" +
			`{"message":{"content":"b"}}` + "\n" +
			`{"done":true}` + "\n",
	)
	var output flushingBuffer

	if err := StreamChatContent(input, &output); err != nil {
		t.Fatalf("StreamChatContent returned error: %v", err)
	}

	if got, want := output.String(), "ab"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if got, want := output.flushes, 2; got != want {
		t.Fatalf("flushes = %d, want %d", got, want)
	}
}

func TestStreamChatContentAndCaptureReturnsFullAssistantMessage(t *testing.T) {
	input := strings.NewReader(
		`{"message":{"content":"hello"}}` + "\n" +
			`{"message":{"content":" again"}}` + "\n" +
			`{"done":true}` + "\n",
	)
	var output flushingBuffer

	got, err := StreamChatContentAndCapture(input, &output)
	if err != nil {
		t.Fatalf("StreamChatContentAndCapture returned error: %v", err)
	}

	if want := "hello again"; got != want {
		t.Fatalf("captured = %q, want %q", got, want)
	}
	if got, want := output.String(), "hello again"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if got, want := output.flushes, 2; got != want {
		t.Fatalf("flushes = %d, want %d", got, want)
	}
}

func TestStreamChatContentReturnsDecodeErrorForInvalidJSON(t *testing.T) {
	input := strings.NewReader(`{"message":`)
	var output bytes.Buffer

	err := StreamChatContent(input, &output)
	if err == nil {
		t.Fatal("StreamChatContent returned nil error, want decode error")
	}
}

func TestNewChatRequestWithMessagesBuildsConversationHistoryPayload(t *testing.T) {
	messages := []chatMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
		{Role: "user", Content: "what did I say?"},
	}

	req, err := NewChatRequestWithMessages("http://localhost:11434/api/chat", "llama3.2", messages)
	if err != nil {
		t.Fatalf("NewChatRequestWithMessages returned error: %v", err)
	}

	var body chatRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if got, want := len(body.Messages), 3; got != want {
		t.Fatalf("message count = %d, want %d", got, want)
	}
	for i, want := range messages {
		if body.Messages[i] != want {
			t.Fatalf("message[%d] = %#v, want %#v", i, body.Messages[i], want)
		}
	}
}

func TestNewChatRequestBuildsStreamingChatPayload(t *testing.T) {
	req, err := NewChatRequest("http://localhost:11434/api/chat", "llama3.2", "say hi")
	if err != nil {
		t.Fatalf("NewChatRequest returned error: %v", err)
	}

	if req.Method != http.MethodPost {
		t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
	}
	if got, want := req.URL.String(), "http://localhost:11434/api/chat"; got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
	if got, want := req.Header.Get("Content-Type"), "application/json"; got != want {
		t.Fatalf("content-type = %q, want %q", got, want)
	}

	var body chatRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if body.Model != "llama3.2" {
		t.Fatalf("model = %q, want llama3.2", body.Model)
	}
	if !body.Stream {
		t.Fatal("stream = false, want true")
	}
	if got, want := len(body.Messages), 1; got != want {
		t.Fatalf("message count = %d, want %d", got, want)
	}
	if got, want := body.Messages[0].Role, "user"; got != want {
		t.Fatalf("message role = %q, want %q", got, want)
	}
	if got, want := body.Messages[0].Content, "say hi"; got != want {
		t.Fatalf("message content = %q, want %q", got, want)
	}
}
