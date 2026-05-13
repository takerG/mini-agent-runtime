package memory

import (
	"context"
	"strings"
)

// Scope 表示 memory 归属范围，user memory 跨会话，session memory 只属于单次会话。
type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeSession Scope = "session"
)

// Query 描述读取或写入 memory 时用于定位用户和会话的键。
type Query struct {
	UserID    string
	SessionID string
}

// Turn 表示一轮已经完成的用户输入和最终助手回答。
type Turn struct {
	User      string
	Assistant string
}

// Block 表示某个 memory provider 贡献给模型的一段上下文。
type Block struct {
	Provider string
	Scope    Scope
	Content  string
}

// Context 是一次模型请求前聚合得到的 memory 上下文。
type Context struct {
	Blocks []Block
}

// SystemMessage 将 memory 上下文格式化成适合作为 system message 注入的文本。
func (c Context) SystemMessage() string {
	if len(c.Blocks) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("Memory context for this request. Use it only when it is relevant to the user request.\n")
	for _, block := range c.Blocks {
		content := strings.TrimSpace(block.Content)
		if content == "" {
			continue
		}
		builder.WriteString("\n[")
		builder.WriteString(string(block.Scope))
		builder.WriteString(":")
		builder.WriteString(block.Provider)
		builder.WriteString("]\n")
		builder.WriteString(content)
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

// Provider 定义 memory 策略需要实现的最小接口。
type Provider interface {
	Name() string
	Scope() Scope
	AppendTurn(ctx context.Context, query Query, turn Turn) error
	ContextBlock(ctx context.Context, query Query) (Block, bool, error)
}

// keyForScope 根据 provider scope 选择 user 级或 session 级存储键。
func keyForScope(scope Scope, query Query) string {
	if scope == ScopeUser {
		return query.UserID
	}
	return query.SessionID
}
