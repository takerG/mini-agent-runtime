package main

import (
	"flag"
	"strings"
	"testing"

	"mini-agent-runtime/internal/agent"
)

func TestParseCLIOptionsAcceptsDoubleDashFlags(t *testing.T) {
	var output strings.Builder

	options, err := parseCLIOptions([]string{
		"--mode", "plan",
		"--trace",
		"--model", "qwen3:4b",
		"hello",
	}, &output)
	if err != nil {
		t.Fatalf("parseCLIOptions returned error: %v", err)
	}

	if got, want := options.mode, agent.ModePlan; got != want {
		t.Fatalf("mode = %q, want %q", got, want)
	}
	if !options.trace {
		t.Fatal("trace = false, want true")
	}
	if got, want := options.model, "qwen3:4b"; got != want {
		t.Fatalf("model = %q, want %q", got, want)
	}
	if got, want := strings.Join(options.initialArgs, " "), "hello"; got != want {
		t.Fatalf("initial args = %q, want %q", got, want)
	}
}

func TestCLIUsageShowsDoubleDashFlags(t *testing.T) {
	var output strings.Builder

	_, err := parseCLIOptions([]string{"--help"}, &output)
	if err != flag.ErrHelp {
		t.Fatalf("parseCLIOptions help error = %v, want %v", err, flag.ErrHelp)
	}

	got := output.String()
	for _, want := range []string{"--mode", "--trace", "--model", "--think"} {
		if !strings.Contains(got, want) {
			t.Fatalf("usage = %q, want to contain %s", got, want)
		}
	}
}
