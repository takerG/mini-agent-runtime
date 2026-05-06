package main

import (
	"flag"
	"net/http"
	"os"
	"strings"

	"mini-agent-runtime/internal/agent"
	apperrors "mini-agent-runtime/internal/errors"
	"mini-agent-runtime/internal/server"
	tracing "mini-agent-runtime/internal/trace"
)

const (
	defaultEndpoint = "http://localhost:11434/api/chat"
	defaultModel    = "qwen3:4b"
)

func main() {
	endpoint := flag.String("url", getenvDefault("LOCAL_MODEL_CHAT_URL", defaultEndpoint), "chat API URL")
	model := flag.String("model", getenvDefault("LOCAL_MODEL_NAME", defaultModel), "local model name")
	think := flag.Bool("think", true, "hide model thinking output when true; show it when false")
	trace := flag.Bool("trace", false, "write agent trace logs to stderr")
	debug := flag.Bool("debug", false, "write structured debug error details to stderr")
	serveAddr := flag.String("serve", "", "serve a streaming HTTP proxy on this address, for example 127.0.0.1:8080")
	flag.Parse()

	if *serveAddr != "" {
		handler := server.NewChatProxyHandler(*endpoint, *model, http.DefaultClient)
		if err := http.ListenAndServe(*serveAddr, handler); err != nil {
			exitWithError(err, *debug)
		}
		return
	}

	err := agent.RunChatLoopWithOptions(agent.ChatLoopOptions{
		Endpoint:    *endpoint,
		Model:       *model,
		Think:       *think,
		Client:      http.DefaultClient,
		InitialArgs: flag.Args(),
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Trace:       tracing.NewTraceHooks(tracing.NewTraceLogger(*trace, os.Stderr)),
		Debug:       *debug,
	})
	if err != nil {
		exitWithError(err, *debug)
	}
}

func getenvDefault(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func exitWithError(err error, debug bool) {
	// main 是程序最外层，只负责把致命错误交给统一 reporter。
	// 真正的发生节点应该由内部包 Wrap 标注；main 这里只代表错误传播到了入口。
	reporter := apperrors.NewReporter(debug, os.Stderr)
	reporter.Log(err)
	reporter.Debug(err)
	os.Exit(1)
}
