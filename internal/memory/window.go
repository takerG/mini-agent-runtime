package memory

import (
	"context"
	"fmt"
	"strings"
)

// WindowMemoryOptions 配置最近 N 轮窗口 memory。
type WindowMemoryOptions struct {
	Scope    Scope
	MaxTurns int
}

// WindowMemory 保存最近 N 轮对话，适合做 session memory。
type WindowMemory struct {
	scope    Scope
	maxTurns int
	turns    map[string][]Turn
}

// NewWindowMemory 创建最近 N 轮窗口 memory。
func NewWindowMemory(options WindowMemoryOptions) *WindowMemory {
	scope := options.Scope
	if scope == "" {
		scope = ScopeSession
	}
	maxTurns := options.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 6
	}
	return &WindowMemory{
		scope:    scope,
		maxTurns: maxTurns,
		turns:    make(map[string][]Turn),
	}
}

// Name 返回窗口 memory 的 provider 名称。
func (m *WindowMemory) Name() string {
	return "recent_window"
}

// Scope 返回窗口 memory 的归属范围。
func (m *WindowMemory) Scope() Scope {
	return m.scope
}

// AppendTurn 追加一轮对话，并裁剪到最近 N 轮。
func (m *WindowMemory) AppendTurn(_ context.Context, query Query, turn Turn) error {
	if m == nil {
		return nil
	}
	key := keyForScope(m.scope, query)
	if key == "" {
		return nil
	}
	m.turns[key] = append(m.turns[key], turn)
	if len(m.turns[key]) > m.maxTurns {
		m.turns[key] = m.turns[key][len(m.turns[key])-m.maxTurns:]
	}
	return nil
}

// ContextBlock 返回最近 N 轮对话的可读上下文。
func (m *WindowMemory) ContextBlock(_ context.Context, query Query) (Block, bool, error) {
	if m == nil {
		return Block{}, false, nil
	}
	key := keyForScope(m.scope, query)
	turns := m.turns[key]
	if len(turns) == 0 {
		return Block{}, false, nil
	}
	var builder strings.Builder
	for i, turn := range turns {
		fmt.Fprintf(&builder, "%d. User: %s\n", i+1, turn.User)
		fmt.Fprintf(&builder, "%d. Assistant: %s\n", i+1, turn.Assistant)
	}
	return Block{
		Provider: m.Name(),
		Scope:    m.scope,
		Content:  strings.TrimSpace(builder.String()),
	}, true, nil
}
