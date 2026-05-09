package model

import (
	"context"
	"fmt"
	"net/http"

	apperrors "mini-agent-runtime/internal/errors"
	"mini-agent-runtime/internal/ollama"
	tracing "mini-agent-runtime/internal/trace"
)

type Options struct {
	Endpoint string
	Model    string
	Think    bool
	HTTP     *http.Client
	Trace    *tracing.TraceHooks
}

type Client struct {
	endpoint string
	model    string
	think    bool
	http     *http.Client
	trace    *tracing.TraceHooks
}

type ChatOptions struct {
	Phase     string
	ToolRound int
	Model     string
	Think     *bool
	Messages  []ollama.Message
	Tools     []ollama.ToolDefinition
	Stream    ollama.StreamOptions
}

type ChatResult struct {
	Content   string
	ToolCalls []ollama.ToolCall
}

func NewClient(options Options) *Client {
	httpClient := options.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	traceHooks := options.Trace
	if traceHooks == nil {
		traceHooks = tracing.NewTraceHooks(nil)
	}
	return &Client{
		endpoint: options.Endpoint,
		model:    options.Model,
		think:    options.Think,
		http:     httpClient,
		trace:    traceHooks,
	}
}

func (c *Client) Chat(ctx context.Context, options ChatOptions) (ChatResult, error) {
	modelName := options.Model
	if modelName == "" {
		modelName = c.model
	}
	think := &c.think
	if options.Think != nil {
		think = options.Think
	}

	req, err := ollama.NewChatRequestWithOptions(ctx, ollama.ChatRequestOptions{
		Endpoint: c.endpoint,
		Model:    modelName,
		Messages: options.Messages,
		Think:    think,
		Tools:    options.Tools,
	})
	if err != nil {
		return ChatResult{}, apperrors.Wrap(apperrors.NodeModelClient, apperrors.CodeRequestBuildFailed, err, "build model request")
	}
	c.trace.ModelRequest(tracing.ModelRequestTrace{
		Phase:     options.Phase,
		ToolRound: options.ToolRound,
		Request:   ollama.NewChatPayload(modelName, options.Messages, think, options.Tools),
	})

	resp, err := c.http.Do(req)
	if err != nil {
		return ChatResult{}, apperrors.Wrap(apperrors.NodeModelClient, apperrors.CodeModelRequestFailed, err, "post model request")
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		resp.Body.Close()
		return ChatResult{}, apperrors.New(apperrors.NodeModelClient, apperrors.CodeUpstreamStatusFailed, fmt.Sprintf("model request failed: %s", resp.Status))
	}

	content, toolCalls, streamErr := ollama.StreamChatMessageAndCaptureWithOptions(resp.Body, options.Stream)
	closeErr := resp.Body.Close()
	if streamErr != nil {
		return ChatResult{}, apperrors.Wrap(apperrors.NodeModelClient, apperrors.CodeModelRequestFailed, streamErr, "stream model response")
	}
	if closeErr != nil {
		return ChatResult{}, apperrors.Wrap(apperrors.NodeModelClient, apperrors.CodeResponseCloseFailed, closeErr, "close model response")
	}
	c.trace.ModelResponse(tracing.ModelResponseTrace{
		Phase:     options.Phase,
		ToolRound: options.ToolRound,
		Content:   content,
		ToolCalls: toolCalls,
	})

	return ChatResult{Content: content, ToolCalls: toolCalls}, nil
}
