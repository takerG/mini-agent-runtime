package memory

import (
	"context"
	"strings"
)

// Summarizer 定义摘要 memory 如何把旧摘要和新对话压缩成新摘要。
type Summarizer interface {
	Summarize(ctx context.Context, existing string, turn Turn) (string, error)
}

// SummarizerFunc 允许普通函数作为摘要策略接入 SummaryMemory。
type SummarizerFunc func(ctx context.Context, existing string, turn Turn) (string, error)

// Summarize 调用函数形式的摘要策略。
func (f SummarizerFunc) Summarize(ctx context.Context, existing string, turn Turn) (string, error) {
	return f(ctx, existing, turn)
}

// LocalSummarizer 使用本地字符串策略生成摘要，适合作为离线 demo 和测试默认实现。
type LocalSummarizer struct{}

// NewLocalSummarizer 创建本地字符串摘要器。
func NewLocalSummarizer() LocalSummarizer {
	return LocalSummarizer{}
}

// Summarize 使用本地字符串追加方式更新摘要。
func (LocalSummarizer) Summarize(_ context.Context, existing string, turn Turn) (string, error) {
	line := "- User: " + turn.User + " | Assistant: " + turn.Assistant
	if strings.TrimSpace(existing) == "" {
		return line, nil
	}
	return existing + "\n" + line, nil
}

// SummaryMemoryOptions 配置摘要 memory 的 scope 和摘要函数。
type SummaryMemoryOptions struct {
	Scope      Scope
	Summarizer Summarizer
}

// SummaryMemory 保存滚动摘要，适合做长期 user memory 或较长 session memory。
type SummaryMemory struct {
	scope      Scope
	summarizer Summarizer
	summaries  map[string]string
}

// NewSummaryMemory 创建摘要 memory。
func NewSummaryMemory(options SummaryMemoryOptions) *SummaryMemory {
	scope := options.Scope
	if scope == "" {
		scope = ScopeUser
	}
	summarizer := options.Summarizer
	if summarizer == nil {
		summarizer = NewLocalSummarizer()
	}
	return &SummaryMemory{
		scope:      scope,
		summarizer: summarizer,
		summaries:  make(map[string]string),
	}
}

// Name 返回摘要 memory 的 provider 名称。
func (m *SummaryMemory) Name() string {
	return "summary"
}

// Scope 返回摘要 memory 的归属范围。
func (m *SummaryMemory) Scope() Scope {
	return m.scope
}

// AppendTurn 使用 summarizer 更新当前 scope 下的摘要。
func (m *SummaryMemory) AppendTurn(ctx context.Context, query Query, turn Turn) error {
	if m == nil {
		return nil
	}
	key := keyForScope(m.scope, query)
	if key == "" {
		return nil
	}
	summary, err := m.summarizer.Summarize(ctx, m.summaries[key], turn)
	if err != nil {
		return err
	}
	m.summaries[key] = strings.TrimSpace(summary)
	return nil
}

// ContextBlock 返回当前摘要上下文。
func (m *SummaryMemory) ContextBlock(_ context.Context, query Query) (Block, bool, error) {
	if m == nil {
		return Block{}, false, nil
	}
	key := keyForScope(m.scope, query)
	summary := strings.TrimSpace(m.summaries[key])
	if summary == "" {
		return Block{}, false, nil
	}
	return Block{
		Provider: m.Name(),
		Scope:    m.scope,
		Content:  summary,
	}, true, nil
}
