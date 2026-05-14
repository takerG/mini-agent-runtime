package agent

import (
	"context"
	"strings"
	"testing"

	"mini-agent-runtime/internal/memory"
)

// TestSessionMessagesForModelInjectsMemoryWithoutMutatingHistory 验证 Session 会在模型请求前注入 memory，但不会污染内部历史。
func TestSessionMessagesForModelInjectsMemoryWithoutMutatingHistory(t *testing.T) {
	query := memory.Query{UserID: "u1", SessionID: "s1"}
	manager := memory.NewManager(memory.NewWindowMemory(memory.WindowMemoryOptions{Scope: memory.ScopeSession, MaxTurns: 2}))
	if err := manager.AppendTurn(context.Background(), query, memory.Turn{User: "remember city", Assistant: "Tokyo"}); err != nil {
		t.Fatalf("AppendTurn returned error: %v", err)
	}

	session := NewSession(SessionOptions{Memory: manager, MemoryQuery: query})
	session.AppendUserMessage("what city?")

	messages, err := session.MessagesForModel(context.Background())
	if err != nil {
		t.Fatalf("MessagesForModel returned error: %v", err)
	}

	if got, want := len(messages), 2; got != want {
		t.Fatalf("model message count = %d, want %d", got, want)
	}
	if got := messages[0]; got.Role != "system" || !strings.Contains(got.Content, "Memory context") || !strings.Contains(got.Content, "remember city") {
		t.Fatalf("memory system message = %#v, want injected memory", got)
	}
	if got := messages[1]; got.Role != "user" || got.Content != "what city?" {
		t.Fatalf("user message = %#v, want user content", got)
	}

	history := session.History()
	if got, want := len(history), 1; got != want {
		t.Fatalf("history count = %d, want %d", got, want)
	}
	if got := history[0]; got.Role != "user" || got.Content != "what city?" {
		t.Fatalf("history message = %#v, want user content", got)
	}
}

// TestSessionCommitTurnWritesMemory 验证 Session 在一轮完成后会统一把最终问答写入 memory。
func TestSessionCommitTurnWritesMemory(t *testing.T) {
	query := memory.Query{UserID: "u1", SessionID: "s1"}
	manager := memory.NewManager(memory.NewWindowMemory(memory.WindowMemoryOptions{Scope: memory.ScopeSession, MaxTurns: 2}))
	session := NewSession(SessionOptions{Memory: manager, MemoryQuery: query})

	if err := session.CommitTurn(context.Background(), "favorite color?", "blue"); err != nil {
		t.Fatalf("CommitTurn returned error: %v", err)
	}

	messages, err := session.MessagesForModel(context.Background())
	if err != nil {
		t.Fatalf("MessagesForModel returned error: %v", err)
	}
	if len(messages) != 1 || !strings.Contains(messages[0].Content, "favorite color?") || !strings.Contains(messages[0].Content, "blue") {
		t.Fatalf("messages = %#v, want memory for committed turn", messages)
	}
}
