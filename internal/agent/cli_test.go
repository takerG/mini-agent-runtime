package agent

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"mini-agent-runtime/internal/ollama"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

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
