package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type flusher interface {
	// Flush 的语义是“把缓冲区里的内容立刻推给对端”。
	// http.ResponseWriter 在支持流式响应时会实现 http.Flusher；
	// 测试里也可以用一个假的 writer 来统计 Flush 被调用了几次。
	Flush()
}

type chatMessage struct {
	// Role 表示消息角色。对 chat API 来说，最常见的是：
	//   system: 系统提示词，定义模型行为
	//   user: 用户输入
	//   assistant: 模型回复
	// 第一版只发送一条 user 消息。
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	// Model 是本地要调用的模型名，例如 qwen2.5、llama3.2 等。
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`

	// Stream=true 是本项目的关键。
	// 如果为 false，服务端通常会等完整答案生成完再一次性返回；
	// 如果为 true，服务端会边生成边返回多行 JSON，客户端就可以边读边输出。
	Stream bool `json:"stream"`

	// Think 是 Ollama 针对部分思考模型的输出控制，例如 qwen3。
	// 按当前实测行为：
	//   true  表示隐藏 think 流，只输出最终回答相关内容；
	//   false 表示显示 think 流，便于观察模型推理过程。
	Think *bool `json:"think,omitempty"`
}

type chatResponse struct {
	// Message.Content 是我们真正要展示给用户的文本片段。
	// 流式响应中，每一行 JSON 通常只包含一小段 content。
	Message chatMessage `json:"message"`

	// Done 通常表示模型是否已经结束输出。
	// 当前实现不依赖 Done 来停止，因为读取到 EOF 时自然结束；
	// 但保留这个字段便于以后做更细的状态处理。
	Done bool `json:"done"`

	// Error 是上游模型服务可能返回的错误信息。
	// 如果流里出现 error，就应该停止转发并把错误返回给调用者。
	Error string `json:"error,omitempty"`
}

func NewChatRequest(endpoint string, model string, userMessage string) (*http.Request, error) {
	// 普通 CLI 调用没有外部请求上下文，因此使用 context.Background()。
	// HTTP 代理模式会使用 NewChatRequestWithContext，把客户端断开连接的信息传进去。
	return NewChatRequestWithContext(context.Background(), endpoint, model, userMessage)
}

func NewChatRequestWithMessages(endpoint string, model string, messages []chatMessage) (*http.Request, error) {
	return NewChatRequestWithMessagesAndContext(context.Background(), endpoint, model, messages)
}

func NewChatRequestWithContext(ctx context.Context, endpoint string, model string, userMessage string) (*http.Request, error) {
	return NewChatRequestWithMessagesAndContext(ctx, endpoint, model, []chatMessage{
		{Role: "user", Content: userMessage},
	})
}

func NewChatRequestWithMessagesAndContext(ctx context.Context, endpoint string, model string, messages []chatMessage) (*http.Request, error) {
	return NewChatRequestWithMessagesThinkAndContext(ctx, endpoint, model, messages, nil)
}

func NewChatRequestWithMessagesThinkAndContext(ctx context.Context, endpoint string, model string, messages []chatMessage, think *bool) (*http.Request, error) {
	// 这里把用户的一句话包装成 Ollama chat API 需要的 JSON 结构。
	// 未来做 agent 时，可以在 Messages 里加入：
	//   1. system prompt
	//   2. 历史对话
	//   3. 工具调用结果
	// 第二版开始，CLI 会把历史 user/assistant 消息都放进 Messages，
	// 让本地模型能看到上下文，从而支持多轮对话。
	payload := chatRequest{
		Model:    model,
		Messages: append([]chatMessage(nil), messages...),
		Stream:   true,
		Think:    think,
	}

	// 用 json.Encoder 写入 bytes.Buffer，比手写字符串安全：
	// 它会自动处理引号、换行、中文等 JSON 转义细节。
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return nil, fmt.Errorf("encode chat request: %w", err)
	}

	// NewRequestWithContext 允许请求跟随 ctx 取消。
	// 在 HTTP 代理里，如果下游用户断开连接，r.Context() 会取消，
	// Go 的 HTTP 客户端也会尽快停止向上游模型读取，避免浪费算力。
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return nil, fmt.Errorf("create chat request: %w", err)
	}

	// 告诉模型服务请求体是 JSON。很多 HTTP API 都会依赖这个头来解析 body。
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func StreamChatContent(r io.Reader, w io.Writer) error {
	_, err := StreamChatContentAndCapture(r, w)
	return err
}

func StreamChatContentAndCapture(r io.Reader, w io.Writer) (string, error) {
	var captured strings.Builder

	// Ollama 的 stream=true 响应是“newline-delimited JSON”：
	// 每一行都是一个完整 JSON 对象，行与行之间用 \n 分隔。
	// bufio.Scanner 很适合处理这种“按行读取”的协议。
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		// scanner.Bytes() 是当前这一行的字节。
		// TrimSpace 可以忽略空行或多余空白，避免 json.Unmarshal 因空行报错。
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		// 每一行都单独解析成 chatResponse。
		// 这一步就是“流式接收”：不等待完整回答，只处理当前刚到的一小块。
		var response chatResponse
		if err := json.Unmarshal(line, &response); err != nil {
			return captured.String(), fmt.Errorf("decode chat response: %w", err)
		}
		if response.Error != "" {
			return captured.String(), fmt.Errorf("chat response error: %s", response.Error)
		}

		// 结束行可能只有 {"done":true}，没有 content。
		// 这种行不需要输出给用户，直接跳过。
		if response.Message.Content == "" {
			continue
		}

		// 写出当前片段。
		// 对 CLI 来说，w 是 os.Stdout；
		// 对 HTTP 代理来说，w 是 http.ResponseWriter。
		if _, err := io.WriteString(w, response.Message.Content); err != nil {
			return captured.String(), fmt.Errorf("write chat content: %w", err)
		}
		captured.WriteString(response.Message.Content)

		// 关键点：写完一个 content chunk 就立即 Flush。
		// 很多 writer/HTTP server 会为了效率先把数据放在缓冲区里；
		// 如果不 Flush，用户可能要等缓冲区满或请求结束后才看到内容。
		// 这里主动 Flush，可以最大化“边生成边显示”的速度体验。
		if flushWriter, ok := w.(flusher); ok {
			flushWriter.Flush()
		}
	}

	// Scanner 循环结束有两种可能：
	//   1. 正常 EOF，上游流结束，没有错误
	//   2. 读取过程中出错，例如网络断开
	// scanner.Err() 用来区分这两种情况。
	if err := scanner.Err(); err != nil {
		return captured.String(), fmt.Errorf("read chat response: %w", err)
	}
	return captured.String(), nil
}
