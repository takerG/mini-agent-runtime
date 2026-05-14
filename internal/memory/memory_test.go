package memory

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	modelclient "mini-agent-runtime/internal/model"
	"mini-agent-runtime/internal/ollama"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip 让测试可以用函数模拟 http.RoundTripper。
func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

// TestWindowMemoryKeepsRecentTurns 验证窗口 memory 只保留最近 N 轮对话。
func TestWindowMemoryKeepsRecentTurns(t *testing.T) {
	store := NewWindowMemory(WindowMemoryOptions{Scope: ScopeSession, MaxTurns: 2})
	query := Query{UserID: "u1", SessionID: "s1"}

	for _, turn := range []Turn{
		{User: "u1", Assistant: "a1"},
		{User: "u2", Assistant: "a2"},
		{User: "u3", Assistant: "a3"},
	} {
		if err := store.AppendTurn(context.Background(), query, turn); err != nil {
			t.Fatalf("AppendTurn returned error: %v", err)
		}
	}

	block, ok, err := store.ContextBlock(context.Background(), query)
	if err != nil {
		t.Fatalf("ContextBlock returned error: %v", err)
	}
	if !ok {
		t.Fatal("ContextBlock ok = false, want true")
	}
	if strings.Contains(block.Content, "u1") || strings.Contains(block.Content, "a1") {
		t.Fatalf("window content = %q, want oldest turn trimmed", block.Content)
	}
	for _, want := range []string{"u2", "a2", "u3", "a3"} {
		if !strings.Contains(block.Content, want) {
			t.Fatalf("window content = %q, want substring %q", block.Content, want)
		}
	}
}

// TestSummaryMemoryUpdatesPerUser 验证摘要 memory 按 user scope 聚合摘要。
func TestSummaryMemoryUpdatesPerUser(t *testing.T) {
	store := NewSummaryMemory(SummaryMemoryOptions{
		Scope: ScopeUser,
		Summarizer: SummarizerFunc(func(_ context.Context, existing string, turn Turn) (string, error) {
			if existing == "" {
				return turn.User + " -> " + turn.Assistant, nil
			}
			return existing + " | " + turn.User + " -> " + turn.Assistant, nil
		}),
	})

	if err := store.AppendTurn(context.Background(), Query{UserID: "u1", SessionID: "s1"}, Turn{User: "likes tea", Assistant: "noted"}); err != nil {
		t.Fatalf("AppendTurn returned error: %v", err)
	}
	if err := store.AppendTurn(context.Background(), Query{UserID: "u1", SessionID: "s2"}, Turn{User: "prefers short answers", Assistant: "noted"}); err != nil {
		t.Fatalf("AppendTurn returned error: %v", err)
	}

	block, ok, err := store.ContextBlock(context.Background(), Query{UserID: "u1", SessionID: "any"})
	if err != nil {
		t.Fatalf("ContextBlock returned error: %v", err)
	}
	if !ok {
		t.Fatal("ContextBlock ok = false, want true")
	}
	for _, want := range []string{"likes tea", "prefers short answers"} {
		if !strings.Contains(block.Content, want) {
			t.Fatalf("summary content = %q, want substring %q", block.Content, want)
		}
	}
}

// TestSummaryMemoryUsesModelSummarizer 验证摘要 memory 可以使用真实模型调用生成滚动摘要。
func TestSummaryMemoryUsesModelSummarizer(t *testing.T) {
	var requests []ollama.ChatRequest
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode model request: %v", err)
			}
			requests = append(requests, body)

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"用户喜欢茶，也偏好简短回答。"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}
	store := NewSummaryMemory(SummaryMemoryOptions{
		Scope: ScopeUser,
		Summarizer: NewModelSummarizer(modelclient.NewClient(modelclient.Options{
			Endpoint: "http://localhost:11434/api/chat",
			Model:    "qwen3:4b",
			Think:    true,
			HTTP:     httpClient,
		})),
	})

	if err := store.AppendTurn(context.Background(), Query{UserID: "u1", SessionID: "s1"}, Turn{User: "我喜欢茶", Assistant: "已记录，后续会用简短回答。"}); err != nil {
		t.Fatalf("AppendTurn returned error: %v", err)
	}

	if got, want := len(requests), 1; got != want {
		t.Fatalf("model request count = %d, want %d", got, want)
	}
	if got, want := requests[0].Model, "qwen3:4b"; got != want {
		t.Fatalf("model = %q, want %q", got, want)
	}
	if len(requests[0].Messages) != 2 {
		t.Fatalf("messages = %#v, want system and user message", requests[0].Messages)
	}
	for _, want := range []string{"memory summarizer", "只输出更新后的摘要"} {
		if !strings.Contains(requests[0].Messages[0].Content, want) {
			t.Fatalf("system prompt = %q, want substring %q", requests[0].Messages[0].Content, want)
		}
	}
	for _, want := range []string{"existing_summary", "我喜欢茶", "已记录"} {
		if !strings.Contains(requests[0].Messages[1].Content, want) {
			t.Fatalf("user prompt = %q, want substring %q", requests[0].Messages[1].Content, want)
		}
	}

	block, ok, err := store.ContextBlock(context.Background(), Query{UserID: "u1", SessionID: "any"})
	if err != nil {
		t.Fatalf("ContextBlock returned error: %v", err)
	}
	if !ok {
		t.Fatal("ContextBlock ok = false, want true")
	}
	if got, want := block.Content, "用户喜欢茶，也偏好简短回答。"; got != want {
		t.Fatalf("summary content = %q, want %q", got, want)
	}
}

// TestDBSessionStateMemoryUsesLocalStore 验证 DB session state 首版只通过本地内存模拟访问函数。
func TestDBSessionStateMemoryUsesLocalStore(t *testing.T) {
	store := NewDBSessionStateMemory()
	query := Query{UserID: "u1", SessionID: "s1"}

	if err := store.Set(context.Background(), "s1", "last_tool", "calculator"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	value, ok, err := store.Get(context.Background(), "s1", "last_tool")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if !ok || value != "calculator" {
		t.Fatalf("Get = %q, %v, want calculator, true", value, ok)
	}

	block, ok, err := store.ContextBlock(context.Background(), query)
	if err != nil {
		t.Fatalf("ContextBlock returned error: %v", err)
	}
	if !ok {
		t.Fatal("ContextBlock ok = false, want true")
	}
	if !strings.Contains(block.Content, "last_tool=calculator") {
		t.Fatalf("state content = %q, want key value", block.Content)
	}
}

// TestManagerComposesMemoryBlocks 验证 manager 可以组合 user memory 和 session memory。
func TestManagerComposesMemoryBlocks(t *testing.T) {
	manager := NewManager(
		NewSummaryMemory(SummaryMemoryOptions{Scope: ScopeUser}),
		NewWindowMemory(WindowMemoryOptions{Scope: ScopeSession, MaxTurns: 1}),
	)
	query := Query{UserID: "u1", SessionID: "s1"}

	if err := manager.AppendTurn(context.Background(), query, Turn{User: "hello", Assistant: "hi"}); err != nil {
		t.Fatalf("AppendTurn returned error: %v", err)
	}
	contextValue, err := manager.Context(context.Background(), query)
	if err != nil {
		t.Fatalf("Context returned error: %v", err)
	}
	message := contextValue.SystemMessage()
	for _, want := range []string{"user", "session", "hello", "hi"} {
		if !strings.Contains(message, want) {
			t.Fatalf("system memory = %q, want substring %q", message, want)
		}
	}
}
