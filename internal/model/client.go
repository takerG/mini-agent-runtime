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
	Phase        string
	ToolRound    int
	Model        string
	Think        *bool
	Messages     []ollama.Message
	Tools        []ollama.ToolDefinition
	Stream       ollama.StreamOptions
	TraceContext tracing.TraceContext
}

type ChatResult struct {
	Content   string
	ToolCalls []ollama.ToolCall
}

// NewClient 创建模型客户端，并为 HTTP client 和 trace hooks 填充默认值。
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

// Chat 向本地模型发送 chat 请求，流式输出内容并捕获最终文本与工具调用。
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
	traceHooks := c.trace.WithContext(options.TraceContext)
	traceHooks.ModelRequest(tracing.ModelRequestTrace{
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
	traceHooks.ModelResponse(tracing.ModelResponseTrace{
		Phase:     options.Phase,
		ToolRound: options.ToolRound,
		Content:   content,
		ToolCalls: toolCalls,
	})

	return ChatResult{Content: content, ToolCalls: toolCalls}, nil
}
