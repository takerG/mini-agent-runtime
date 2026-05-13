package agent

import (
	"context"

	apperrors "mini-agent-runtime/internal/errors"
	"mini-agent-runtime/internal/memory"
	"mini-agent-runtime/internal/ollama"
)

// SessionOptions 描述创建会话状态时需要注入的 memory 依赖和定位键。
type SessionOptions struct {
	Memory      *memory.Manager
	MemoryQuery memory.Query
}

// Session 保存一次 CLI 会话中的消息历史，并统一管理每轮结束后的 memory 写入。
type Session struct {
	messages    []ollama.Message
	memory      *memory.Manager
	memoryQuery memory.Query
}

// NewSession 创建会话状态对象，并为未显式传入的 memory 依赖补齐默认值。
func NewSession(options SessionOptions) *Session {
	memoryManager := options.Memory
	if memoryManager == nil {
		memoryManager = memory.NewDefaultManager()
	}
	return &Session{
		messages:    []ollama.Message{},
		memory:      memoryManager,
		memoryQuery: defaultMemoryQuery(options.MemoryQuery),
	}
}

// AppendUserMessage 把用户输入追加到当前会话历史中。
func (s *Session) AppendUserMessage(content string) {
	s.messages = append(s.messages, ollama.Message{Role: "user", Content: content})
}

// AppendAssistantMessage 把助手最终回答追加到当前会话历史中。
func (s *Session) AppendAssistantMessage(content string) {
	s.messages = append(s.messages, ollama.Message{Role: "assistant", Content: content})
}

// AppendAssistantToolCallMessage 把包含工具调用的助手消息追加到当前会话历史中。
func (s *Session) AppendAssistantToolCallMessage(content string, toolCalls []ollama.ToolCall) {
	s.messages = append(s.messages, ollama.Message{
		Role:      "assistant",
		Content:   content,
		ToolCalls: toolCalls,
	})
}

// AppendToolMessage 把工具执行结果追加到当前会话历史中，供下一轮模型请求继续观察。
func (s *Session) AppendToolMessage(toolName string, content string) {
	s.messages = append(s.messages, ollama.Message{
		Role:     "tool",
		Content:  content,
		ToolName: toolName,
	})
}

// History 返回当前会话消息历史的副本，避免外部调用方直接修改内部状态。
func (s *Session) History() []ollama.Message {
	return append([]ollama.Message(nil), s.messages...)
}

// MessagesForModel 读取当前 memory 上下文，并把它作为 system message 注入到消息历史副本前面。
func (s *Session) MessagesForModel(ctx context.Context) ([]ollama.Message, error) {
	memoryContext, err := s.memory.Context(ctx, s.memoryQuery)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.NodeAgentLoop, apperrors.CodeUnknown, err, "read memory context")
	}
	return messagesWithMemory(memoryContext, s.messages), nil
}

// CommitTurn 在一轮对话完成后，把用户输入和最终助手回答写入所有 memory provider。
func (s *Session) CommitTurn(ctx context.Context, userMessage string, assistantMessage string) error {
	if err := s.memory.AppendTurn(ctx, s.memoryQuery, memory.Turn{User: userMessage, Assistant: assistantMessage}); err != nil {
		return apperrors.Wrap(apperrors.NodeAgentLoop, apperrors.CodeUnknown, err, "append memory turn")
	}
	return nil
}
