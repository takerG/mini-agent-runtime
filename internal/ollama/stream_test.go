package ollama

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

type flushingBuffer struct {
	bytes.Buffer
	flushes int
}

// Flush 记录测试 writer 被流式处理逻辑刷新了一次。
func (b *flushingBuffer) Flush() {
	b.flushes++
}

// TestStreamChatContentWritesMessageContentChunks 验证流式解析会写出每个 message.content chunk。
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

// TestStreamChatContentFlushesAfterEachMessageContentChunk 验证每次内容写入后都会触发 Flush。
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

// TestStreamChatContentHandlesLongJSONLine 验证单行 JSON 超过 bufio.Scanner 默认上限时仍可正常流式解析。
func TestStreamChatContentHandlesLongJSONLine(t *testing.T) {
	longContent := strings.Repeat("x", 70*1024)
	payload, err := json.Marshal(ChatResponse{Message: Message{Content: longContent}})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	input := bytes.NewReader(append(payload, '\n'))
	var output bytes.Buffer

	if err := StreamChatContent(input, &output); err != nil {
		t.Fatalf("StreamChatContent returned error: %v", err)
	}
	if got := output.String(); got != longContent {
		t.Fatalf("output length = %d, want %d", len(got), len(longContent))
	}
}

// TestStreamChatContentAndCaptureReturnsFullAssistantMessage 验证流式输出的同时能捕获完整助手文本。
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

// TestStreamChatMessageBeforeContentHookRunsOnceBeforeFirstWrite 验证首个内容写入前 hook 只执行一次。
func TestStreamChatMessageBeforeContentHookRunsOnceBeforeFirstWrite(t *testing.T) {
	input := strings.NewReader(
		`{"message":{"content":"a"}}` + "\n" +
			`{"message":{"content":"b"}}` + "\n" +
			`{"done":true}` + "\n",
	)
	var events []string
	writer := writerFunc(func(p []byte) (int, error) {
		events = append(events, "write:"+string(p))
		return len(p), nil
	})

	content, _, err := StreamChatMessageAndCaptureWithOptions(input, StreamOptions{
		Writer: writer,
		BeforeContent: func() error {
			events = append(events, "before")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("StreamChatMessageAndCaptureWithOptions returned error: %v", err)
	}

	if got, want := content, "ab"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	wantEvents := []string{"before", "write:a", "write:b"}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", events, wantEvents)
	}
}

// TestStreamChatContentReturnsDecodeErrorForInvalidJSON 验证非法 JSON 会返回解码错误。
func TestStreamChatContentReturnsDecodeErrorForInvalidJSON(t *testing.T) {
	input := strings.NewReader(`{"message":`)
	var output bytes.Buffer

	err := StreamChatContent(input, &output)
	if err == nil {
		t.Fatal("StreamChatContent returned nil error, want decode error")
	}
}

type writerFunc func([]byte) (int, error)

// Write 让测试可以用函数模拟 io.Writer。
func (fn writerFunc) Write(p []byte) (int, error) {
	return fn(p)
}

// TestNewChatRequestWithMessagesBuildsConversationHistoryPayload 验证多轮消息会进入请求负载。
func TestNewChatRequestWithMessagesBuildsConversationHistoryPayload(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
		{Role: "user", Content: "what did I say?"},
	}

	req, err := NewChatRequestWithMessages("http://localhost:11434/api/chat", "llama3.2", messages)
	if err != nil {
		t.Fatalf("NewChatRequestWithMessages returned error: %v", err)
	}

	var body ChatRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if got, want := len(body.Messages), 3; got != want {
		t.Fatalf("message count = %d, want %d", got, want)
	}
	for i, want := range messages {
		if !reflect.DeepEqual(body.Messages[i], want) {
			t.Fatalf("message[%d] = %#v, want %#v", i, body.Messages[i], want)
		}
	}
}

// TestNewChatRequestBuildsStreamingChatPayload 验证单轮请求会构造流式 chat 负载。
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

	var body ChatRequest
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
