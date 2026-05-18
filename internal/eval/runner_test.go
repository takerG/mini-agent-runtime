package eval

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mini-agent-runtime/internal/agent"
	"mini-agent-runtime/internal/approval"
	"mini-agent-runtime/internal/ollama"
	"mini-agent-runtime/internal/tools"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip 让测试可以用函数模拟 http.RoundTripper。
func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

// TestLoadSuiteParsesCasesAndModes 验证 eval suite 可以从 JSON 文件读取用例和目标模式。
func TestLoadSuiteParsesCasesAndModes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "suite.json")
	content := `{
  "name": "unit",
  "cases": [
    {
      "name": "calculator",
      "input": "23 * 19 等于多少？",
      "modes": ["chat", "strict-plan"],
      "expect": {
        "tool_calls": [{"name": "calculator"}],
        "answer_contains": ["437"]
      }
    }
  ]
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write suite: %v", err)
	}

	suite, err := LoadSuite(path)
	if err != nil {
		t.Fatalf("LoadSuite returned error: %v", err)
	}

	if got, want := suite.Name, "unit"; got != want {
		t.Fatalf("suite name = %q, want %q", got, want)
	}
	if got, want := suite.Cases[0].Modes[1], agent.ModeStrictPlan; got != want {
		t.Fatalf("case mode = %q, want %q", got, want)
	}
	if got, want := suite.Cases[0].Expect.ToolCalls[0].Name, "calculator"; got != want {
		t.Fatalf("expected tool = %q, want %q", got, want)
	}
}

// TestLoadSuiteParsesStructuredExpectations 验证 eval suite 可以读取结构化工具结果和 observation 断言。
func TestLoadSuiteParsesStructuredExpectations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "suite.json")
	content := `{
  "name": "structured",
  "cases": [
    {
      "name": "calculator",
      "input": "23 * 19 等于多少？",
      "mode": "chat",
      "expect": {
        "tool_results": [
          {
            "name": "calculator",
            "result_contains": ["437"]
          }
        ],
        "observations": [
          {
            "type": "tool_result",
            "name": "calculator",
            "content_contains": ["437"]
          }
        ]
      }
    }
  ]
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write suite: %v", err)
	}

	suite, err := LoadSuite(path)
	if err != nil {
		t.Fatalf("LoadSuite returned error: %v", err)
	}

	if got, want := suite.Cases[0].Expect.ToolResults[0].ResultContains[0], "437"; got != want {
		t.Fatalf("tool result contains = %q, want %q", got, want)
	}
	if got, want := suite.Cases[0].Expect.Observations[0].Type, "tool_result"; got != want {
		t.Fatalf("observation type = %q, want %q", got, want)
	}
}

// TestRunnerRunsChatCaseAndChecksStructuredToolData 验证 eval runner 可以运行 chat 模式并检查结构化工具数据和最终答案。
func TestRunnerRunsChatCaseAndChecksStructuredToolData(t *testing.T) {
	requests := 0
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			if requests == 1 {
				return chatResponse(`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"calculator","arguments":{"op":"*","a":23,"b":19}}}]},"done":true}`), nil
			}
			return chatResponse(`{"message":{"content":"23 * 19 = 437"}}`), nil
		}),
	}
	runner := NewRunner(RunnerOptions{
		Endpoint: "http://localhost:11434/api/chat",
		Model:    "qwen3:4b",
		Think:    true,
		Client:   client,
		Output:   io.Discard,
		Tools:    tools.NewDefaultToolRegistry(nil),
	})
	suite := Suite{
		Name: "chat",
		Cases: []Case{
			{
				Name:  "calculator",
				Input: "23 * 19 等于多少？",
				Mode:  agent.ModeChat,
				Expect: Expectation{
					ToolCalls:      []ExpectedToolCall{{Name: "calculator"}},
					ToolResults:    []ExpectedToolResult{{Name: "calculator", ResultContains: []string{"437"}}},
					Observations:   []ExpectedObservation{{Type: "tool_result", Name: "calculator", ContentContains: []string{"437"}}},
					AnswerContains: []string{"437"},
				},
			},
		},
	}

	report, err := runner.RunSuite(context.Background(), suite)
	if err != nil {
		t.Fatalf("RunSuite returned error: %v", err)
	}

	if got, want := report.Passed, 1; got != want {
		t.Fatalf("passed = %d, want %d; report=%#v", got, want, report)
	}
	if got, want := report.Failed, 0; got != want {
		t.Fatalf("failed = %d, want %d; report=%#v", got, want, report)
	}
}

// TestRunnerRunsStrictPlanCaseAndChecksGoToolExecution 验证 strict-plan 模式会通过 Go 执行工具并检查结果。
func TestRunnerRunsStrictPlanCaseAndChecksGoToolExecution(t *testing.T) {
	var requests []ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body ollama.ChatRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			requests = append(requests, body)
			if len(requests) == 1 {
				return chatResponse(`{"message":{"content":"{\"goal\":\"calculate\",\"steps\":[{\"id\":\"step1\",\"type\":\"tool_call\",\"tool_name\":\"calculator\",\"arguments\":{\"op\":\"*\",\"a\":23,\"b\":19}}]}"}}`), nil
			}
			return chatResponse(`{"message":{"content":"计算结果是 437。"}}`), nil
		}),
	}
	runner := NewRunner(RunnerOptions{
		Endpoint: "http://localhost:11434/api/chat",
		Model:    "qwen3:4b",
		Think:    true,
		Client:   client,
		Output:   io.Discard,
		Tools:    tools.NewDefaultToolRegistry(nil),
	})
	suite := Suite{
		Name: "strict",
		Cases: []Case{
			{
				Name:  "strict calculator",
				Input: "计算 23 * 19",
				Mode:  agent.ModeStrictPlan,
				Expect: Expectation{
					ToolCalls:      []ExpectedToolCall{{Name: "calculator"}},
					ToolCallCount:  intPtr(1),
					AnswerContains: []string{"437"},
				},
			},
		},
	}

	report, err := runner.RunSuite(context.Background(), suite)
	if err != nil {
		t.Fatalf("RunSuite returned error: %v", err)
	}

	if got, want := report.Passed, 1; got != want {
		t.Fatalf("passed = %d, want %d; report=%#v", got, want, report)
	}
	if got, want := len(requests), 2; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
	if got, want := len(requests[1].Tools), 0; got != want {
		t.Fatalf("summary tools = %d, want %d", got, want)
	}
}

// TestRunnerChecksApprovalEvents 验证 eval runner 可以基于结构化 trace 断言审批请求和决策。
func TestRunnerChecksApprovalEvents(t *testing.T) {
	requests := 0
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests++
			if requests == 1 {
				return chatResponse(`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"dangerous_operation","arguments":{"action":"删除生产数据"}}}]},"done":true}`), nil
			}
			return chatResponse(`{"message":{"content":"已完成高危操作模拟"}}`), nil
		}),
	}
	runner := NewRunner(RunnerOptions{
		Endpoint: "http://localhost:11434/api/chat",
		Model:    "qwen3:4b",
		Think:    true,
		Client:   client,
		Output:   io.Discard,
		Tools:    tools.NewDefaultToolRegistry(nil),
		ToolPolicy: tools.ExecutionPolicy{
			MaxAttempts: 1,
			Approval: approval.Policy{
				Gate: approval.NewAutoGate(approval.Decision{Status: approval.StatusApproved, Approver: "eval"}),
			},
		},
	})
	suite := Suite{
		Name: "approval",
		Cases: []Case{
			{
				Name:  "dangerous operation",
				Input: "请执行高危操作",
				Mode:  agent.ModeChat,
				Expect: Expectation{
					ApprovalRequests:  []ExpectedApprovalRequest{{ToolName: "dangerous_operation", RiskLevel: "high"}},
					ApprovalDecisions: []ExpectedApprovalDecision{{ToolName: "dangerous_operation", Decision: "approved", Approver: "eval"}},
					ToolResults:       []ExpectedToolResult{{Name: "dangerous_operation", ResultContains: []string{"【高危操作】"}}},
					AnswerContains:    []string{"高危操作"},
				},
			},
		},
	}

	report, err := runner.RunSuite(context.Background(), suite)
	if err != nil {
		t.Fatalf("RunSuite returned error: %v", err)
	}

	if got, want := report.Passed, 1; got != want {
		t.Fatalf("passed = %d, want %d; report=%#v", got, want, report)
	}
}

// chatResponse 构造单行 Ollama 流式响应。
func chatResponse(line string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(line + "\n")),
	}
}

// intPtr 返回 int 指针，便于构造带可选计数断言的测试用例。
func intPtr(value int) *int {
	return &value
}
