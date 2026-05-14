package ollama

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	apperrors "mini-agent-runtime/internal/errors"
)

type flusher interface {
	Flush()
}

// StreamChatContent 从 Ollama 流式响应中解析 message.content，并立即写入目标 writer。
func StreamChatContent(r io.Reader, w io.Writer) error {
	_, err := StreamChatContentAndCapture(r, w)
	return err
}

// StreamChatContentAndCapture 流式转发内容的同时返回完整捕获文本。
func StreamChatContentAndCapture(r io.Reader, w io.Writer) (string, error) {
	content, _, err := StreamChatMessageAndCapture(r, w)
	return content, err
}

// StreamChatMessageAndCapture 流式转发内容，并捕获完整文本和模型返回的工具调用。
func StreamChatMessageAndCapture(r io.Reader, w io.Writer) (string, []ToolCall, error) {
	return StreamChatMessageAndCaptureWithOptions(r, StreamOptions{Writer: w})
}

type StreamOptions struct {
	Writer        io.Writer
	BeforeContent func() error
}

// StreamChatMessageAndCaptureWithOptions 使用扩展选项解析 Ollama 流式响应。
func StreamChatMessageAndCaptureWithOptions(r io.Reader, options StreamOptions) (string, []ToolCall, error) {
	var captured strings.Builder
	var toolCalls []ToolCall
	contentStarted := false
	w := options.Writer
	if w == nil {
		w = io.Discard
	}

	reader := bufio.NewReader(r)
	for {
		rawLine, readErr := reader.ReadBytes('\n')
		line := bytes.TrimSpace(rawLine)
		if len(line) == 0 {
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					break
				}
				return captured.String(), toolCalls, apperrors.Wrap(apperrors.NodeOllamaStream, apperrors.CodeStreamReadFailed, readErr, "read chat response")
			}
			continue
		}

		var response ChatResponse
		if err := json.Unmarshal(line, &response); err != nil {
			return captured.String(), toolCalls, apperrors.Wrap(apperrors.NodeOllamaStream, apperrors.CodeStreamDecodeFailed, err, "decode chat response")
		}
		if response.Error != "" {
			return captured.String(), toolCalls, apperrors.New(apperrors.NodeOllamaStream, apperrors.CodeUpstreamRequestFailed, fmt.Sprintf("chat response error: %s", response.Error))
		}
		if len(response.Message.ToolCalls) > 0 {
			toolCalls = append(toolCalls, response.Message.ToolCalls...)
		}
		if response.Message.Content != "" {
			if !contentStarted && options.BeforeContent != nil {
				if err := options.BeforeContent(); err != nil {
					return captured.String(), toolCalls, apperrors.Wrap(apperrors.NodeOllamaStream, apperrors.CodeStreamWriteFailed, err, "run before content hook")
				}
			}
			contentStarted = true

			if _, err := io.WriteString(w, response.Message.Content); err != nil {
				return captured.String(), toolCalls, apperrors.Wrap(apperrors.NodeOllamaStream, apperrors.CodeStreamWriteFailed, err, "write chat content")
			}
			captured.WriteString(response.Message.Content)

			if flushWriter, ok := w.(flusher); ok {
				flushWriter.Flush()
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return captured.String(), toolCalls, apperrors.Wrap(apperrors.NodeOllamaStream, apperrors.CodeStreamReadFailed, readErr, "read chat response")
		}
	}
	return captured.String(), toolCalls, nil
}
