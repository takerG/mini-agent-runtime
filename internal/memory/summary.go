package memory

import (
	"context"
	"strings"
)

// Summarizer 定义摘要 memory 如何把旧摘要和新对话压缩成新摘要。
type Summarizer func(existing string, turn Turn) string

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
		summarizer = defaultSummarizer
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
func (m *SummaryMemory) AppendTurn(_ context.Context, query Query, turn Turn) error {
	if m == nil {
		return nil
	}
	key := keyForScope(m.scope, query)
	if key == "" {
		return nil
	}
	m.summaries[key] = strings.TrimSpace(m.summarizer(m.summaries[key], turn))
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

// defaultSummarizer 使用本地字符串压缩模拟摘要更新，未来可替换成模型摘要。
func defaultSummarizer(existing string, turn Turn) string {
	line := "- User: " + turn.User + " | Assistant: " + turn.Assistant
	if strings.TrimSpace(existing) == "" {
		return line
	}
	return existing + "\n" + line
}
