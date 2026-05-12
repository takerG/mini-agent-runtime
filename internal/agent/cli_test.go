package agent

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
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

// TestRunChatLoopSendsConversationHistoryAcrossTurns 验证多轮 CLI 会把历史消息继续传给模型。
func TestRunChatLoopSendsConversationHistoryAcrossTurns(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			answer := "answer one"
			if len(requests) == 2 {
				answer = "answer two"
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"` + answer + `"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoop(
		"http://localhost:11434/api/chat",
		"llama3.2",
		true,
		client,
		nil,
		strings.NewReader("first\nsecond\n/exit\n"),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("RunChatLoop returned error: %v", err)
	}

	if got, want := len(requests), 2; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	if got, want := len(requests[0].Messages), 1; got != want {
		t.Fatalf("first request message count = %d, want %d", got, want)
	}
	if got, want := requests[0].Messages[0], (ollama.Message{Role: "user", Content: "first"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("first request message = %#v, want %#v", got, want)
	}
	if requests[0].Think == nil || !*requests[0].Think {
		t.Fatalf("first request think = %v, want true", requests[0].Think)
	}

	wantHistory := []ollama.Message{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "answer one"},
		{Role: "user", Content: "second"},
	}
	if got := requests[1].Messages; len(got) != len(wantHistory) {
		t.Fatalf("second request message count = %d, want %d", len(got), len(wantHistory))
	}
	for i, want := range wantHistory {
		if !reflect.DeepEqual(requests[1].Messages[i], want) {
			t.Fatalf("second request message[%d] = %#v, want %#v", i, requests[1].Messages[i], want)
		}
	}
	if got, want := stdout.String(), "answer one\nanswer two\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// TestRunChatLoopUsesArgsAsFirstMessageThenContinuesReadingStdin 验证启动入参会作为首轮消息且后续继续读取 stdin。
func TestRunChatLoopUsesArgsAsFirstMessageThenContinuesReadingStdin(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"ok"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoop(
		"http://localhost:11434/api/chat",
		"llama3.2",
		true,
		client,
		[]string{"from", "args"},
		strings.NewReader("/exit\n"),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("RunChatLoop returned error: %v", err)
	}

	if got, want := len(requests), 1; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	if got, want := requests[0].Messages[0], (ollama.Message{Role: "user", Content: "from args"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("first request message = %#v, want %#v", got, want)
	}
}

// TestRunChatLoopUsesConfiguredThinkValue 验证 CLI 会把 think 参数传入模型请求。
func TestRunChatLoopUsesConfiguredThinkValue(t *testing.T) {
	var request ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"ok"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoop(
		"http://localhost:11434/api/chat",
		"llama3.2",
		false,
		client,
		[]string{"hello"},
		strings.NewReader("/exit\n"),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("RunChatLoop returned error: %v", err)
	}

	if request.Think == nil {
		t.Fatal("request think = nil, want false")
	}
	if *request.Think {
		t.Fatal("request think = true, want false")
	}
}

// TestRunChatLoopSendsToolDefinitions 验证普通对话模式会把工具定义发送给模型。
func TestRunChatLoopSendsToolDefinitions(t *testing.T) {
	var request ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"ok"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoop(
		"http://localhost:11434/api/chat",
		"llama3.2",
		true,
		client,
		[]string{"what time is it?"},
		strings.NewReader("/exit\n"),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("RunChatLoop returned error: %v", err)
	}

	if got, want := len(request.Tools), 2; got != want {
		t.Fatalf("tool count = %d, want %d", got, want)
	}
	toolNames := map[string]bool{}
	for _, tool := range request.Tools {
		toolNames[tool.Function.Name] = true
	}
	for _, want := range []string{"current_time", "calculator"} {
		if !toolNames[want] {
			t.Fatalf("tool names = %v, want %q", toolNames, want)
		}
	}
}

// TestRunChatLoopExecutesCalculatorToolCallThenAsksModelForFinalAnswer 验证模型工具调用会被执行并进入下一轮总结。
func TestRunChatLoopExecutesCalculatorToolCallThenAsksModelForFinalAnswer(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			if len(requests) == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"calculator","arguments":{"op":"+","a":2,"b":3}}}]},"done":true}` + "\n",
					)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"2+3=5"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoop(
		"http://localhost:11434/api/chat",
		"llama3.2",
		true,
		client,
		[]string{"2+3等于多少？"},
		strings.NewReader("/exit\n"),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("RunChatLoop returned error: %v", err)
	}

	if got, want := len(requests), 2; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	secondMessages := requests[1].Messages
	if got, want := len(secondMessages), 3; got != want {
		t.Fatalf("second request message count = %d, want %d", got, want)
	}
	if got, want := secondMessages[1].ToolCalls[0].Function.Name, "calculator"; got != want {
		t.Fatalf("assistant tool call name = %q, want %q", got, want)
	}
	if got, want := secondMessages[2].Role, "tool"; got != want {
		t.Fatalf("tool result role = %q, want %q", got, want)
	}
	if got, want := secondMessages[2].ToolName, "calculator"; got != want {
		t.Fatalf("tool result name = %q, want %q", got, want)
	}
	if got, want := secondMessages[2].Content, "5"; got != want {
		t.Fatalf("tool result content = %q, want %q", got, want)
	}
	if got, want := stdout.String(), "2+3=5\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// TestRunChatLoopReturnsUnknownToolErrorToModel 验证未知工具错误会作为 observation 返回给模型。
func TestRunChatLoopReturnsUnknownToolErrorToModel(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			if len(requests) == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"missing_tool","arguments":{}}}]},"done":true}` + "\n",
					)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"tool unavailable"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoop(
		"http://localhost:11434/api/chat",
		"llama3.2",
		true,
		client,
		[]string{"use a missing tool"},
		strings.NewReader("/exit\n"),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("RunChatLoop returned error: %v", err)
	}

	if got, want := len(requests), 2; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	toolMessage := requests[1].Messages[2]
	if got, want := toolMessage.Role, "tool"; got != want {
		t.Fatalf("tool message role = %q, want %q", got, want)
	}
	if got, want := toolMessage.ToolName, "missing_tool"; got != want {
		t.Fatalf("tool message name = %q, want %q", got, want)
	}
	for _, want := range []string{
		"tool error:",
		"code=tool_execution_failed",
		"origin=tools.registry",
		"node_chain=agent.tool_call > tools.registry",
		"detail=unknown tool: missing_tool",
	} {
		if !strings.Contains(toolMessage.Content, want) {
			t.Fatalf("tool message content = %q, want substring %q", toolMessage.Content, want)
		}
	}
	if got, want := stdout.String(), "tool unavailable\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// TestRunChatLoopReturnsToolExecutionErrorToModel 验证工具执行错误不会中断对话而是交给模型处理。
func TestRunChatLoopReturnsToolExecutionErrorToModel(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			if len(requests) == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"calculator","arguments":{"op":"/","a":8,"b":0}}}]},"done":true}` + "\n",
					)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"cannot divide by zero"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoop(
		"http://localhost:11434/api/chat",
		"llama3.2",
		true,
		client,
		[]string{"8/0"},
		strings.NewReader("/exit\n"),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatalf("RunChatLoop returned error: %v", err)
	}

	if got, want := len(requests), 2; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	toolMessage := requests[1].Messages[2]
	if got, want := toolMessage.Role, "tool"; got != want {
		t.Fatalf("tool message role = %q, want %q", got, want)
	}
	if got, want := toolMessage.ToolName, "calculator"; got != want {
		t.Fatalf("tool message name = %q, want %q", got, want)
	}
	for _, want := range []string{
		"tool error:",
		"code=tool_execution_failed",
		"origin=tools.calculator",
		"node_chain=agent.tool_call > tools.calculator",
		"detail=division by zero",
	} {
		if !strings.Contains(toolMessage.Content, want) {
			t.Fatalf("tool message content = %q, want substring %q", toolMessage.Content, want)
		}
	}
	if got, want := stdout.String(), "cannot divide by zero\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// TestRunChatLoopDebugPrintsToolErrorDetails 验证 debug 模式会打印工具错误的结构化细节。
func TestRunChatLoopDebugPrintsToolErrorDetails(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			if len(requests) == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"missing_tool","arguments":{}}}]},"done":true}` + "\n",
					)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"handled"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoopWithOptions(ChatLoopOptions{
		Endpoint:    "http://localhost:11434/api/chat",
		Model:       "llama3.2",
		Think:       true,
		Client:      client,
		InitialArgs: []string{"use a missing tool"},
		Stdin:       strings.NewReader("/exit\n"),
		Stdout:      &stdout,
		Stderr:      &stderr,
		Debug:       true,
	})
	if err != nil {
		t.Fatalf("RunChatLoopWithOptions returned error: %v", err)
	}

	got := stderr.String()
	for _, want := range []string{
		"[debug] error:",
		"code=tool_execution_failed",
		"origin=tools.registry",
		"node_chain=agent.tool_call > tools.registry",
		"detail=unknown tool: missing_tool",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr = %q, want substring %q", got, want)
		}
	}
}

// TestRunChatLoopPlannerExecutorModePlansThenExecutesWithTools 验证 plan 模式会先规划再通过原生工具调用执行。
func TestRunChatLoopPlannerExecutorModePlansThenExecutesWithTools(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			switch len(requests) {
			case 1:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"{\"goal\":\"answer calculation\",\"steps\":[{\"task\":\"calculate 23*19\",\"tool_hint\":\"calculator\"}]}"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			case 2:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"calculator","arguments":{"op":"*","a":23,"b":19}}}]},"done":true}` + "\n",
					)),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"23*19=437"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			}
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoopWithOptions(ChatLoopOptions{
		Endpoint:    "http://localhost:11434/api/chat",
		Model:       "qwen3:4b",
		Think:       true,
		Client:      client,
		InitialArgs: []string{"23 * 19?"},
		Stdin:       strings.NewReader("/exit\n"),
		Stdout:      &stdout,
		Stderr:      &stderr,
		Mode:        ModePlan,
	})
	if err != nil {
		t.Fatalf("RunChatLoopWithOptions returned error: %v", err)
	}

	if got, want := len(requests), 3; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	if got, want := len(requests[0].Tools), 0; got != want {
		t.Fatalf("planner request tool count = %d, want %d", got, want)
	}
	if got, want := len(requests[1].Tools), 2; got != want {
		t.Fatalf("executor request tool count = %d, want %d", got, want)
	}
	if !strings.Contains(requests[1].Messages[0].Content, "calculate 23*19") {
		t.Fatalf("executor system prompt = %q, want plan step", requests[1].Messages[0].Content)
	}
	wantOutput := strings.Join([]string{
		"[plan]",
		`1. tool_call calculator {"a":23,"b":19,"op":"*"}`,
		"",
		"[observation]",
		"1. calculator -> 437",
		"",
		"Agent:",
		"23*19=437",
		"",
	}, "\n")
	if got := stdout.String(); got != wantOutput {
		t.Fatalf("stdout = %q, want %q", got, wantOutput)
	}
}

// TestRunChatLoopStrictPlannerExecutorModeExecutesToolsInGo 验证 strict-plan 模式由 Go 直接执行可执行计划中的工具。
func TestRunChatLoopStrictPlannerExecutorModeExecutesToolsInGo(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			switch len(requests) {
			case 1:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"{\"goal\":\"answer calculation\",\"steps\":[{\"type\":\"tool_call\",\"tool_name\":\"calculator\",\"arguments\":{\"op\":\"*\",\"a\":23,\"b\":19}}]}"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"23*19=437"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			}
		}),
	}

	var stdout strings.Builder
	var stderr strings.Builder
	err := RunChatLoopWithOptions(ChatLoopOptions{
		Endpoint:    "http://localhost:11434/api/chat",
		Model:       "qwen3:4b",
		Think:       true,
		Client:      client,
		InitialArgs: []string{"23 * 19?"},
		Stdin:       strings.NewReader("/exit\n"),
		Stdout:      &stdout,
		Stderr:      &stderr,
		Mode:        ModeStrictPlan,
	})
	if err != nil {
		t.Fatalf("RunChatLoopWithOptions returned error: %v", err)
	}

	if got, want := len(requests), 2; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	if got, want := len(requests[0].Tools), 0; got != want {
		t.Fatalf("planner request tool count = %d, want %d", got, want)
	}
	if got, want := len(requests[1].Tools), 0; got != want {
		t.Fatalf("summary request tool count = %d, want %d", got, want)
	}
	if got := stdout.String(); !strings.Contains(got, "[plan]") || !strings.Contains(got, "calculator -> 437") || !strings.Contains(got, "23*19=437") {
		t.Fatalf("stdout = %q, want strict process output", got)
	}
}

// TestRuntimeRunsPlannerExecutorTurnWithSharedDependencies 验证 Runtime 在 planner/executor 流程中复用共享依赖。
func TestRuntimeRunsPlannerExecutorTurnWithSharedDependencies(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			switch len(requests) {
			case 1:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"{\"goal\":\"answer calculation\",\"steps\":[{\"task\":\"calculate 23*19\",\"tool_hint\":\"calculator\"}]}"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			case 2:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"calculator","arguments":{"op":"*","a":23,"b":19}}}]},"done":true}` + "\n",
					)),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"23*19=437"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			}
		}),
	}

	var stdout strings.Builder
	runtime := NewRuntime(RuntimeOptions{
		ModelClient: modelclient.NewClient(modelclient.Options{
			Endpoint: "http://localhost:11434/api/chat",
			Model:    "qwen3:4b",
			Think:    true,
			HTTP:     client,
		}),
		Tools:  tools.NewDefaultToolRegistry(nil),
		Trace:  tracing.NewTraceHooks(nil),
		Stdout: &stdout,
	})

	answer, err := runtime.RunPlannerExecutorTurn(t.Context(), "23 * 19?")
	if err != nil {
		t.Fatalf("RunPlannerExecutorTurn returned error: %v", err)
	}
	if got, want := answer, "23*19=437"; got != want {
		t.Fatalf("answer = %q, want %q", got, want)
	}
	if got, want := len(requests), 3; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	if got := stdout.String(); !strings.Contains(got, "[plan]") || !strings.Contains(got, "23*19=437") {
		t.Fatalf("stdout = %q, want visible process and final answer", got)
	}
}

// TestRuntimeRunsStrictPlannerExecutorTurnWithoutModelToolCalls 验证 strict-plan 流程不会让模型二次决定工具调用。
func TestRuntimeRunsStrictPlannerExecutorTurnWithoutModelToolCalls(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			switch len(requests) {
			case 1:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"{\"goal\":\"make record\",\"steps\":[{\"type\":\"tool_call\",\"tool_name\":\"calculator\",\"arguments\":{\"a\":23,\"b\":19,\"op\":\"*\"}},{\"type\":\"tool_call\",\"tool_name\":\"current_time\",\"arguments\":{}}]}"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"计算结果是 437，当前时间是 2026-05-09 10:20:30 CST。"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			}
		}),
	}

	var stdout strings.Builder
	runtime := NewRuntime(RuntimeOptions{
		ModelClient: modelclient.NewClient(modelclient.Options{
			Endpoint: "http://localhost:11434/api/chat",
			Model:    "qwen3:4b",
			Think:    true,
			HTTP:     client,
		}),
		Tools: tools.NewDefaultToolRegistry(func() time.Time {
			return time.Date(2026, 5, 9, 10, 20, 30, 0, time.FixedZone("CST", 8*60*60))
		}),
		Trace:  tracing.NewTraceHooks(nil),
		Stdout: &stdout,
	})

	answer, err := runtime.RunStrictPlannerExecutorTurn(t.Context(), "请先计算 23 * 19，再获取当前时间。")
	if err != nil {
		t.Fatalf("RunStrictPlannerExecutorTurn returned error: %v", err)
	}

	if got, want := answer, "计算结果是 437，当前时间是 2026-05-09 10:20:30 CST。"; got != want {
		t.Fatalf("answer = %q, want %q", got, want)
	}
	if got, want := len(requests), 2; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	if got, want := len(requests[0].Tools), 0; got != want {
		t.Fatalf("strict planner request tool count = %d, want %d", got, want)
	}
	if got, want := len(requests[1].Tools), 0; got != want {
		t.Fatalf("strict summary request tool count = %d, want %d", got, want)
	}
	if !strings.Contains(requests[1].Messages[0].Content, "observations") {
		t.Fatalf("summary system prompt = %q, want observations", requests[1].Messages[0].Content)
	}

	wantOutput := strings.Join([]string{
		"[plan]",
		`1. tool_call calculator {"a":23,"b":19,"op":"*"}`,
		"2. tool_call current_time {}",
		"",
		"[observation]",
		"1. calculator -> 437",
		"2. current_time -> 2026-05-09 10:20:30 CST",
		"",
		"Agent:",
		"计算结果是 437，当前时间是 2026-05-09 10:20:30 CST。",
	}, "\n")
	if got := stdout.String(); got != wantOutput {
		t.Fatalf("stdout = %q, want %q", got, wantOutput)
	}
}

// TestRuntimeStrictPlannerExecutorFeedsToolErrorsIntoObservations 验证 strict-plan 工具错误会进入 observation。
func TestRuntimeStrictPlannerExecutorFeedsToolErrorsIntoObservations(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode upstream request body: %v", err)
			}
			requests = append(requests, body)

			if len(requests) == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"message":{"content":"{\"goal\":\"bad tool\",\"steps\":[{\"type\":\"tool_call\",\"tool_name\":\"missing_tool\",\"arguments\":{}}]}"}}` + "\n" +
							`{"done":true}` + "\n",
					)),
				}, nil
			}

			if !strings.Contains(requests[1].Messages[0].Content, "tool error:") {
				t.Fatalf("summary prompt = %q, want tool error observation", requests[1].Messages[0].Content)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"工具不可用。"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	var stdout strings.Builder
	runtime := NewRuntime(RuntimeOptions{
		ModelClient: modelclient.NewClient(modelclient.Options{
			Endpoint: "http://localhost:11434/api/chat",
			Model:    "qwen3:4b",
			Think:    true,
			HTTP:     client,
		}),
		Tools:  tools.NewDefaultToolRegistry(nil),
		Trace:  tracing.NewTraceHooks(nil),
		Stdout: &stdout,
	})

	_, err := runtime.RunStrictPlannerExecutorTurn(t.Context(), "use missing tool")
	if err != nil {
		t.Fatalf("RunStrictPlannerExecutorTurn returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "unknown tool: missing_tool") {
		t.Fatalf("stdout = %q, want unknown tool observation", stdout.String())
	}
}

// TestStrictPromptsRequireCompleteToolPlanAndNoInventedObservations 验证 strict-plan 提示词约束计划完整性和总结边界。
func TestStrictPromptsRequireCompleteToolPlanAndNoInventedObservations(t *testing.T) {
	plannerPrompt := strictPlannerSystemPrompt()
	for _, want := range []string{
		"Every required external fact or calculation must become a tool_call",
		"Use current_time",
		"Use calculator",
	} {
		if !strings.Contains(plannerPrompt, want) {
			t.Fatalf("strict planner prompt = %q, want substring %q", plannerPrompt, want)
		}
	}

	summaryPrompt := strictSummarySystemPrompt(planner.ExecutablePlan{
		Goal: "answer",
		Steps: []planner.ExecutableStep{
			{Type: planner.ExecutableStepToolCall, ToolName: "calculator", Arguments: map[string]any{"a": 23, "b": 19, "op": "*"}},
		},
	}, []strictObservation{{ToolName: "calculator", Result: "437"}})
	if !strings.Contains(summaryPrompt, "Do not invent missing observations") {
		t.Fatalf("strict summary prompt = %q, want no-invention instruction", summaryPrompt)
	}
}
