package memory

import (
	"context"
	"strings"
	"testing"
)

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
		Summarizer: func(existing string, turn Turn) string {
			if existing == "" {
				return turn.User + " -> " + turn.Assistant
			}
			return existing + " | " + turn.User + " -> " + turn.Assistant
		},
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
