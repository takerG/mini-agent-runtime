package executor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	modelclient "mini-agent-runtime/internal/model"
	"mini-agent-runtime/internal/ollama"
	"mini-agent-runtime/internal/planner"
	"mini-agent-runtime/internal/tools"
	tracing "mini-agent-runtime/internal/trace"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip 让测试可以用函数模拟 http.RoundTripper。
func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

// TestExecutorRunsPlanWithNativeToolCalls 验证 executor 会根据计划驱动模型原生工具调用。
func TestExecutorRunsPlanWithNativeToolCalls(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode executor request: %v", err)
			}
			requests = append(requests, body)

			if len(requests) == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"calculator","arguments":{"op":"*","a":23,"b":19}}}]},"done":true}` + "\n",
					)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"23*19=437"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	executor := NewExecutor(Options{
		ModelClient: modelclient.NewClient(modelclient.Options{
			Endpoint: "http://localhost:11434/api/chat",
			Model:    "qwen3:4b",
			Think:    true,
			HTTP:     client,
		}),
		Dependencies: Dependencies{
			Registry: tools.NewDefaultToolRegistry(time.Now),
			Trace:    tracing.NewTraceHooks(nil),
			Stdout:   &stdout,
		},
	})

	answer, err := executor.Execute(context.Background(), "23 * 19?", planner.Plan{
		Goal: "answer calculation",
		Steps: []planner.Step{
			{Task: "calculate 23*19", ToolHint: "calculator"},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if got, want := answer, "23*19=437"; got != want {
		t.Fatalf("answer = %q, want %q", got, want)
	}
	if got, want := stdout.String(), "23*19=437"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got, want := len(requests), 2; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	if got, want := len(requests[0].Tools), 3; got != want {
		t.Fatalf("executor tool count = %d, want %d", got, want)
	}
	if !strings.Contains(requests[0].Messages[0].Content, "calculate 23*19") {
		t.Fatalf("executor system prompt = %q, want plan step", requests[0].Messages[0].Content)
	}
	toolMessage := requests[1].Messages[3]
	if got, want := toolMessage.Role, "tool"; got != want {
		t.Fatalf("tool message role = %q, want %q", got, want)
	}
	if got, want := toolMessage.Content, "437"; got != want {
		t.Fatalf("tool result = %q, want %q", got, want)
	}
}

// TestExecutorCanPrintVisiblePlanObservationsAndFinalAnswer 验证 executor 能输出可见计划、观测和最终回答。
func TestExecutorCanPrintVisiblePlanObservationsAndFinalAnswer(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode executor request: %v", err)
			}
			requests = append(requests, body)

			if len(requests) == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"calculator","arguments":{"op":"*","a":23,"b":19}}},{"function":{"name":"current_time","arguments":{}}}]},"done":true}` + "\n",
					)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"计算结果是 437，当前时间是 2026-05-08 20:51:42 CST。"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	executor := NewExecutor(Options{
		ModelClient: modelclient.NewClient(modelclient.Options{
			Endpoint: "http://localhost:11434/api/chat",
			Model:    "qwen3:4b",
			Think:    true,
			HTTP:     client,
		}),
		Dependencies: Dependencies{
			Registry: tools.NewDefaultToolRegistry(func() time.Time {
				return time.Date(2026, 5, 8, 20, 51, 42, 0, time.FixedZone("CST", 8*60*60))
			}),
			Trace:  tracing.NewTraceHooks(nil),
			Stdout: &stdout,
		},
		ShowProcess: true,
	})

	_, err := executor.Execute(context.Background(), "make a record", planner.Plan{
		Goal: "make a short record",
		Steps: []planner.Step{
			{Task: "calculate 23*19", ToolHint: "calculator"},
			{Task: "get current time", ToolHint: "current_time"},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	want := strings.Join([]string{
		"[plan]",
		`1. tool_call calculator {"a":23,"b":19,"op":"*"}`,
		"2. tool_call current_time {}",
		"",
		"[observation]",
		"1. calculator -> 437",
		"2. current_time -> 2026-05-08 20:51:42 CST",
		"",
		"Agent:",
		"计算结果是 437，当前时间是 2026-05-08 20:51:42 CST。",
	}, "\n")
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// TestExecutorAggregatesVisibleProcessAcrossToolRounds 验证多轮工具调用的过程输出会聚合展示。
func TestExecutorAggregatesVisibleProcessAcrossToolRounds(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode executor request: %v", err)
			}
			requests = append(requests, body)

			switch len(requests) {
			case 1:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"calculator","arguments":{"op":"*","a":23,"b":19}}}]},"done":true}` + "\n",
					)),
				}, nil
			case 2:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"current_time","arguments":{}}}]},"done":true}` + "\n",
					)),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"计算结果是 437，当前时间是 2026-05-08 20:51:42 CST。"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			}
		}),
	}

	var stdout strings.Builder
	executor := NewExecutor(Options{
		ModelClient: modelclient.NewClient(modelclient.Options{
			Endpoint: "http://localhost:11434/api/chat",
			Model:    "qwen3:4b",
			Think:    true,
			HTTP:     client,
		}),
		Dependencies: Dependencies{
			Registry: tools.NewDefaultToolRegistry(func() time.Time {
				return time.Date(2026, 5, 8, 20, 51, 42, 0, time.FixedZone("CST", 8*60*60))
			}),
			Trace:  tracing.NewTraceHooks(nil),
			Stdout: &stdout,
		},
		ShowProcess: true,
	})

	_, err := executor.Execute(context.Background(), "make a record", planner.Plan{
		Goal: "make a short record",
		Steps: []planner.Step{
			{Task: "calculate 23*19", ToolHint: "calculator"},
			{Task: "get current time", ToolHint: "current_time"},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	want := strings.Join([]string{
		"[plan]",
		`1. tool_call calculator {"a":23,"b":19,"op":"*"}`,
		"2. tool_call current_time {}",
		"",
		"[observation]",
		"1. calculator -> 437",
		"2. current_time -> 2026-05-08 20:51:42 CST",
		"",
		"Agent:",
		"计算结果是 437，当前时间是 2026-05-08 20:51:42 CST。",
	}, "\n")
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}
