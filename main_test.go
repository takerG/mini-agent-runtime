package main

import (
	"errors"
	"flag"
	"net/http"
	"strings"
	"testing"
	"time"

	"mini-agent-runtime/internal/agent"
)

// TestParseCLIOptionsAcceptsDoubleDashFlags 验证 CLI 能解析常见的双横线参数形式。
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

// TestParseCLIOptionsAcceptsTraceJSONLFlag 验证 CLI 可以解析 JSONL trace 文件输出参数。
func TestParseCLIOptionsAcceptsTraceJSONLFlag(t *testing.T) {
	var output strings.Builder

	options, err := parseCLIOptions([]string{
		"--trace-jsonl", "trace.jsonl",
		"hello",
	}, &output)
	if err != nil {
		t.Fatalf("parseCLIOptions returned error: %v", err)
	}
	if got, want := options.traceJSONL, "trace.jsonl"; got != want {
		t.Fatalf("traceJSONL = %q, want %q", got, want)
	}
	if got, want := strings.Join(options.initialArgs, " "), "hello"; got != want {
		t.Fatalf("initial args = %q, want %q", got, want)
	}
}

// TestCLIUsageShowsDoubleDashFlags 验证帮助信息使用双横线展示参数名称。
func TestCLIUsageShowsDoubleDashFlags(t *testing.T) {
	var output strings.Builder

	_, err := parseCLIOptions([]string{"--help"}, &output)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("parseCLIOptions help error = %v, want %v", err, flag.ErrHelp)
	}

	got := output.String()
	for _, want := range []string{"--mode", "--trace", "--trace-jsonl", "--model", "--think"} {
		if !strings.Contains(got, want) {
			t.Fatalf("usage = %q, want to contain %s", got, want)
		}
	}
}

// TestNewHTTPServerConfiguresTimeouts 验证 HTTP proxy server 会显式配置超时，避免默认 server 缺少连接保护。
func TestNewHTTPServerConfiguresTimeouts(t *testing.T) {
	server := newHTTPServer("127.0.0.1:0", http.NotFoundHandler())

	if got, want := server.Addr, "127.0.0.1:0"; got != want {
		t.Fatalf("server addr = %q, want %q", got, want)
	}
	for name, value := range map[string]time.Duration{
		"ReadHeaderTimeout": server.ReadHeaderTimeout,
		"ReadTimeout":       server.ReadTimeout,
		"WriteTimeout":      server.WriteTimeout,
		"IdleTimeout":       server.IdleTimeout,
	} {
		if value <= 0 {
			t.Fatalf("%s = %s, want positive timeout", name, value)
		}
	}
}
