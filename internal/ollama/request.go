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

func NewChatRequest(endpoint string, model string, userMessage string) (*http.Request, error) {
	return NewChatRequestWithContext(context.Background(), endpoint, model, userMessage)
}

func NewChatRequestWithMessages(endpoint string, model string, messages []Message) (*http.Request, error) {
	return NewChatRequestWithMessagesAndContext(context.Background(), endpoint, model, messages)
}

func NewChatRequestWithContext(ctx context.Context, endpoint string, model string, userMessage string) (*http.Request, error) {
	return NewChatRequestWithMessagesAndContext(ctx, endpoint, model, []Message{
		{Role: "user", Content: userMessage},
	})
}

func NewChatRequestWithMessagesAndContext(ctx context.Context, endpoint string, model string, messages []Message) (*http.Request, error) {
	return NewChatRequestWithMessagesThinkAndContext(ctx, endpoint, model, messages, nil)
}

func NewChatRequestWithMessagesThinkAndContext(ctx context.Context, endpoint string, model string, messages []Message, think *bool) (*http.Request, error) {
	return NewChatRequestWithMessagesThinkToolsAndContext(ctx, endpoint, model, messages, think, nil)
}

func NewChatRequestWithMessagesThinkToolsAndContext(ctx context.Context, endpoint string, model string, messages []Message, think *bool, tools []ToolDefinition) (*http.Request, error) {
	return NewChatRequestWithOptions(ctx, ChatRequestOptions{
		Endpoint: endpoint,
		Model:    model,
		Messages: messages,
		Think:    think,
		Tools:    tools,
	})
}

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
