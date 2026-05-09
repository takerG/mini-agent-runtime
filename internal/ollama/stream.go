package ollama

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	apperrors "mini-agent-runtime/internal/errors"
)

type flusher interface {
	Flush()
}

func StreamChatContent(r io.Reader, w io.Writer) error {
	_, err := StreamChatContentAndCapture(r, w)
	return err
}

func StreamChatContentAndCapture(r io.Reader, w io.Writer) (string, error) {
	content, _, err := StreamChatMessageAndCapture(r, w)
	return content, err
}

func StreamChatMessageAndCapture(r io.Reader, w io.Writer) (string, []ToolCall, error) {
	return StreamChatMessageAndCaptureWithOptions(r, StreamOptions{Writer: w})
}

type StreamOptions struct {
	Writer        io.Writer
	BeforeContent func() error
}

func StreamChatMessageAndCaptureWithOptions(r io.Reader, options StreamOptions) (string, []ToolCall, error) {
	var captured strings.Builder
	var toolCalls []ToolCall
	contentStarted := false
	w := options.Writer
	if w == nil {
		w = io.Discard
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
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
		if response.Message.Content == "" {
			continue
		}

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

	if err := scanner.Err(); err != nil {
		return captured.String(), toolCalls, apperrors.Wrap(apperrors.NodeOllamaStream, apperrors.CodeStreamReadFailed, err, "read chat response")
	}
	return captured.String(), toolCalls, nil
}
