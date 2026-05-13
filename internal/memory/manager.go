package memory

import "context"

// Manager 组合多个 memory provider，并向 agent 暴露统一读写入口。
type Manager struct {
	providers []Provider
}

// NewManager 创建 memory manager，provider 的顺序就是最终注入模型的上下文顺序。
func NewManager(providers ...Provider) *Manager {
	return &Manager{providers: append([]Provider(nil), providers...)}
}

// NewDefaultManager 创建默认 memory 组合，首版包含 user summary、session window 和 session state。
func NewDefaultManager() *Manager {
	return NewManager(
		NewSummaryMemory(SummaryMemoryOptions{Scope: ScopeUser}),
		NewWindowMemory(WindowMemoryOptions{Scope: ScopeSession, MaxTurns: 6}),
		NewDBSessionStateMemory(),
	)
}

// AppendTurn 将一轮完整对话写入所有 provider。
func (m *Manager) AppendTurn(ctx context.Context, query Query, turn Turn) error {
	if m == nil {
		return nil
	}
	for _, provider := range m.providers {
		if err := provider.AppendTurn(ctx, query, turn); err != nil {
			return err
		}
	}
	return nil
}

// Context 聚合所有 provider 当前能提供的 memory block。
func (m *Manager) Context(ctx context.Context, query Query) (Context, error) {
	if m == nil {
		return Context{}, nil
	}
	var blocks []Block
	for _, provider := range m.providers {
		block, ok, err := provider.ContextBlock(ctx, query)
		if err != nil {
			return Context{}, err
		}
		if ok {
			blocks = append(blocks, block)
		}
	}
	return Context{Blocks: blocks}, nil
}

// Providers 返回当前 manager 持有的 provider 副本，便于测试或未来 UI 展示。
func (m *Manager) Providers() []Provider {
	if m == nil {
		return nil
	}
	return append([]Provider(nil), m.providers...)
}
