package planner

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	modelclient "mini-agent-runtime/internal/model"
	"mini-agent-runtime/internal/ollama"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestPlannerGeneratesPlanFromModelJSON(t *testing.T) {
	var request ollama.ChatRequest
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				t.Fatalf("decode planner request: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"{\"goal\":\"answer user question\",\"steps\":[{\"task\":\"get current time\",\"tool_hint\":\"current_time\"},{\"task\":\"calculate 23*19\",\"tool_hint\":\"calculator\"}]}"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}

	planner := NewPlanner(Options{
		ModelClient: modelclient.NewClient(modelclient.Options{
			Endpoint: "http://localhost:11434/api/chat",
			Model:    "qwen3:4b",
			Think:    true,
			HTTP:     client,
		}),
	})
	plan, err := planner.Plan("what time is it and what is 23*19?")
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	if got, want := plan.Goal, "answer user question"; got != want {
		t.Fatalf("goal = %q, want %q", got, want)
	}
	if got, want := len(plan.Steps), 2; got != want {
		t.Fatalf("step count = %d, want %d", got, want)
	}
	if got, want := plan.Steps[0].ToolHint, "current_time"; got != want {
		t.Fatalf("first tool hint = %q, want %q", got, want)
	}
	if got, want := len(request.Tools), 0; got != want {
		t.Fatalf("planner tool count = %d, want %d", got, want)
	}
	if got, want := request.Messages[0].Role, "system"; got != want {
		t.Fatalf("planner first role = %q, want %q", got, want)
	}
	if !strings.Contains(request.Messages[0].Content, "JSON") {
		t.Fatalf("planner system prompt = %q, want JSON instruction", request.Messages[0].Content)
	}
}

func TestPlannerParsesFencedJSON(t *testing.T) {
	got, err := ParsePlan("```json\n{\"goal\":\"g\",\"steps\":[{\"task\":\"t\"}]}\n```")
	if err != nil {
		t.Fatalf("ParsePlan returned error: %v", err)
	}
	if got.Goal != "g" || len(got.Steps) != 1 || got.Steps[0].Task != "t" {
		t.Fatalf("plan = %#v, want parsed fenced JSON", got)
	}
}
