package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"mini-agent-runtime/internal/memory"
	modelclient "mini-agent-runtime/internal/model"
	"mini-agent-runtime/internal/ollama"
	"mini-agent-runtime/internal/tools"
	tracing "mini-agent-runtime/internal/trace"
)

// TestRuntimePlannerExecutorTurnCommitsMemory 验证直接调用 runtime 的 plan 模式也会在单轮结束后写入 memory。
func TestRuntimePlannerExecutorTurnCommitsMemory(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			if len(requests) == 1 {
				return chatContentResponse(`{"goal":"remember plan turn","steps":[{"task":"answer directly"}]}`), nil
			}
			return chatContentResponse("planner memory answer"), nil
		}),
	}
	query := memory.Query{UserID: "u1", SessionID: "s1"}
	manager := memory.NewManager(memory.NewWindowMemory(memory.WindowMemoryOptions{Scope: memory.ScopeSession, MaxTurns: 2}))
	runtime := NewRuntime(RuntimeOptions{
		ModelClient: modelclient.NewClient(modelclient.Options{
			Endpoint: "http://localhost:11434/api/chat",
			Model:    "qwen3:4b",
			Think:    true,
			HTTP:     client,
		}),
		Tools:       tools.NewDefaultToolRegistry(nil),
		Trace:       tracing.NewTraceHooks(nil),
		Stdout:      io.Discard,
		Memory:      manager,
		MemoryQuery: query,
	})

	answer, err := runtime.RunPlannerExecutorTurn(context.Background(), "remember this plan turn")
	if err != nil {
		t.Fatalf("RunPlannerExecutorTurn returned error: %v", err)
	}
	if got, want := answer, "planner memory answer"; got != want {
		t.Fatalf("answer = %q, want %q", got, want)
	}

	assertMemoryContainsTurn(t, manager, query, "remember this plan turn", "planner memory answer")
}

// TestRuntimeStrictPlannerExecutorTurnCommitsMemory 验证直接调用 runtime 的 strict-plan 模式也会在单轮结束后写入 memory。
func TestRuntimeStrictPlannerExecutorTurnCommitsMemory(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			if len(requests) == 1 {
				return chatContentResponse(`{"goal":"unsupported request","steps":[]}`), nil
			}
			return chatContentResponse("strict memory answer"), nil
		}),
	}
	query := memory.Query{UserID: "u1", SessionID: "s1"}
	manager := memory.NewManager(memory.NewWindowMemory(memory.WindowMemoryOptions{Scope: memory.ScopeSession, MaxTurns: 2}))
	runtime := NewRuntime(RuntimeOptions{
		ModelClient: modelclient.NewClient(modelclient.Options{
			Endpoint: "http://localhost:11434/api/chat",
			Model:    "qwen3:4b",
			Think:    true,
			HTTP:     client,
		}),
		Tools:       tools.NewDefaultToolRegistry(nil),
		Trace:       tracing.NewTraceHooks(nil),
		Stdout:      io.Discard,
		Memory:      manager,
		MemoryQuery: query,
	})

	answer, err := runtime.RunStrictPlannerExecutorTurn(context.Background(), "remember this strict turn")
	if err != nil {
		t.Fatalf("RunStrictPlannerExecutorTurn returned error: %v", err)
	}
	if got, want := answer, "strict memory answer"; got != want {
		t.Fatalf("answer = %q, want %q", got, want)
	}

	assertMemoryContainsTurn(t, manager, query, "remember this strict turn", "strict memory answer")
}

// chatContentResponse 创建一段 Ollama 兼容的最小流式响应。
func chatContentResponse(content string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(
			`{"message":{"content":` + strconv.Quote(content) + `}}` + "\n" +
				`{"done":true}` + "\n",
		)),
	}
}

// assertMemoryContainsTurn 验证 memory 上下文包含指定的一轮用户输入和助手回答。
func assertMemoryContainsTurn(t *testing.T, manager *memory.Manager, query memory.Query, userMessage string, assistantMessage string) {
	t.Helper()

	contextValue, err := manager.Context(context.Background(), query)
	if err != nil {
		t.Fatalf("memory Context returned error: %v", err)
	}
	systemMessage := contextValue.SystemMessage()
	if !strings.Contains(systemMessage, userMessage) || !strings.Contains(systemMessage, assistantMessage) {
		t.Fatalf("memory context = %q, want user %q and assistant %q", systemMessage, userMessage, assistantMessage)
	}
}
