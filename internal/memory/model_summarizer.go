package memory

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	apperrors "mini-agent-runtime/internal/errors"
	modelclient "mini-agent-runtime/internal/model"
	"mini-agent-runtime/internal/ollama"
	"mini-agent-runtime/internal/prompts"
)

// ModelSummarizerOptions 描述模型摘要器需要的依赖和可选模型覆盖。
type ModelSummarizerOptions struct {
	ModelClient *modelclient.Client
	Model       string
	Think       *bool
}

// ModelSummarizer 使用真实模型把旧摘要和新一轮对话压缩成新的滚动摘要。
type ModelSummarizer struct {
	modelClient *modelclient.Client
	model       string
	think       *bool
}

// NewModelSummarizer 使用传入模型客户端创建默认模型摘要器。
func NewModelSummarizer(modelClient *modelclient.Client) *ModelSummarizer {
	return NewModelSummarizerWithOptions(ModelSummarizerOptions{ModelClient: modelClient})
}

// NewModelSummarizerWithOptions 使用完整配置创建模型摘要器。
func NewModelSummarizerWithOptions(options ModelSummarizerOptions) *ModelSummarizer {
	return &ModelSummarizer{
		modelClient: options.ModelClient,
		model:       options.Model,
		think:       options.Think,
	}
}

// NewModelSummaryManager 创建使用模型摘要的默认 memory 组合。
func NewModelSummaryManager(modelClient *modelclient.Client) *Manager {
	return NewManager(
		NewSummaryMemory(SummaryMemoryOptions{Scope: ScopeUser, Summarizer: NewModelSummarizer(modelClient)}),
		NewWindowMemory(WindowMemoryOptions{Scope: ScopeSession, MaxTurns: 6}),
		NewDBSessionStateMemory(),
	)
}

// Summarize 调用模型生成新的 memory 摘要。
func (s *ModelSummarizer) Summarize(ctx context.Context, existing string, turn Turn) (string, error) {
	if s == nil || s.modelClient == nil {
		return "", apperrors.New(apperrors.NodeMemory, apperrors.CodeModelRequestFailed, "model summarizer requires model client")
	}
	payload := modelSummaryPayload{
		ExistingSummary: strings.TrimSpace(existing),
		NewTurn:         turn,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", apperrors.Wrap(apperrors.NodeMemory, apperrors.CodeModelRequestFailed, err, "marshal memory summary payload")
	}

	result, err := s.modelClient.Chat(ctx, modelclient.ChatOptions{
		Phase: "memory_summary",
		Model: s.model,
		Think: s.think,
		Messages: []ollama.Message{
			{Role: "system", Content: prompts.MemorySummarySystemPrompt()},
			{Role: "user", Content: string(data)},
		},
		Stream: ollama.StreamOptions{Writer: io.Discard},
	})
	if err != nil {
		return "", apperrors.Wrap(apperrors.NodeMemory, apperrors.CodeModelRequestFailed, err, "summarize memory with model")
	}
	summary := strings.TrimSpace(result.Content)
	if summary == "" {
		return "", apperrors.New(apperrors.NodeMemory, apperrors.CodeModelRequestFailed, "model summarizer returned empty summary")
	}
	return summary, nil
}

type modelSummaryPayload struct {
	ExistingSummary string `json:"existing_summary"`
	NewTurn         Turn   `json:"new_turn"`
}
