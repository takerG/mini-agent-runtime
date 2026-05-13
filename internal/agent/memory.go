package agent

import (
	"mini-agent-runtime/internal/memory"
	"mini-agent-runtime/internal/ollama"
)

const (
	defaultMemoryUserID    = "local-user"
	defaultMemorySessionID = "cli-session"
)

// defaultMemoryQuery 补齐 memory 查询键，避免 CLI 使用者必须通过启动参数控制 memory。
func defaultMemoryQuery(query memory.Query) memory.Query {
	if query.UserID == "" {
		query.UserID = defaultMemoryUserID
	}
	if query.SessionID == "" {
		query.SessionID = defaultMemorySessionID
	}
	return query
}

// messagesWithMemory 将 memory context 作为 system message 注入模型请求。
func messagesWithMemory(memoryContext memory.Context, messages []ollama.Message) []ollama.Message {
	systemMessage := memoryContext.SystemMessage()
	if systemMessage == "" {
		return append([]ollama.Message(nil), messages...)
	}
	withMemory := make([]ollama.Message, 0, len(messages)+1)
	withMemory = append(withMemory, ollama.Message{Role: "system", Content: systemMessage})
	withMemory = append(withMemory, messages...)
	return withMemory
}
