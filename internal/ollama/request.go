package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	apperrors "mini-agent-runtime/internal/errors"
)

type ChatRequestOptions struct {
	Endpoint string
	Model    string
	Messages []Message
	Think    *bool
	Tools    []ToolDefinition
}

// NewChatRequestWithOptions 根据完整请求配置创建 Ollama chat HTTP 请求。
func NewChatRequestWithOptions(ctx context.Context, options ChatRequestOptions) (*http.Request, error) {
	payload := NewChatPayload(options.Model, options.Messages, options.Think, options.Tools)

	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return nil, apperrors.Wrap(apperrors.NodeOllamaClient, apperrors.CodeRequestBuildFailed, err, "encode chat request")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, options.Endpoint, &body)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.NodeOllamaClient, apperrors.CodeRequestBuildFailed, err, "create chat request")
	}

	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// NewChatRequest 使用单条用户消息创建默认上下文的 Ollama chat 请求。
func NewChatRequest(endpoint string, model string, userMessage string) (*http.Request, error) {
	return NewChatRequestWithContext(context.Background(), endpoint, model, userMessage)
}

// NewChatRequestWithMessages 使用多轮消息创建默认上下文的 Ollama chat 请求。
func NewChatRequestWithMessages(endpoint string, model string, messages []Message) (*http.Request, error) {
	return NewChatRequestWithMessagesAndContext(context.Background(), endpoint, model, messages)
}

// NewChatRequestWithContext 使用调用方传入的 context 和单条用户消息创建请求。
func NewChatRequestWithContext(ctx context.Context, endpoint string, model string, userMessage string) (*http.Request, error) {
	return NewChatRequestWithMessagesAndContext(ctx, endpoint, model, []Message{
		{Role: "user", Content: userMessage},
	})
}

// NewChatRequestWithMessagesAndContext 使用调用方传入的 context 和完整消息历史创建请求。
func NewChatRequestWithMessagesAndContext(ctx context.Context, endpoint string, model string, messages []Message) (*http.Request, error) {
	return NewChatRequestWithMessagesThinkAndContext(ctx, endpoint, model, messages, nil)
}

// NewChatRequestWithMessagesThinkAndContext 在消息历史请求中显式配置 think 参数。
func NewChatRequestWithMessagesThinkAndContext(ctx context.Context, endpoint string, model string, messages []Message, think *bool) (*http.Request, error) {
	return NewChatRequestWithMessagesThinkToolsAndContext(ctx, endpoint, model, messages, think, nil)
}

// NewChatRequestWithMessagesThinkToolsAndContext 在消息历史请求中同时配置 think 参数和工具定义。
func NewChatRequestWithMessagesThinkToolsAndContext(ctx context.Context, endpoint string, model string, messages []Message, think *bool, tools []ToolDefinition) (*http.Request, error) {
	return NewChatRequestWithOptions(ctx, ChatRequestOptions{
		Endpoint: endpoint,
		Model:    model,
		Messages: messages,
		Think:    think,
		Tools:    tools,
	})
}

// NewChatPayload 创建可用于 trace 或 HTTP body 的 Ollama chat 请求负载副本。
func NewChatPayload(model string, messages []Message, think *bool, tools []ToolDefinition) ChatRequest {
	var thinkCopy *bool
	if think != nil {
		value := *think
		thinkCopy = &value
	}
	return ChatRequest{
		Model:    model,
		Messages: append([]Message(nil), messages...),
		Stream:   true,
		Think:    thinkCopy,
		Tools:    append([]ToolDefinition(nil), tools...),
	}
}
