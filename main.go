package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"mini-agent-runtime/internal/agent"
	apperrors "mini-agent-runtime/internal/errors"
	"mini-agent-runtime/internal/server"
	tracing "mini-agent-runtime/internal/trace"
)

// cliOptions 保存入口层解析后的 CLI 配置。
type cliOptions struct {
	endpoint    string
	model       string
	think       bool
	trace       bool
	debug       bool
	mode        agent.Mode
	serveAddr   string
	initialArgs []string
}

const (
	defaultEndpoint = "http://localhost:11434/api/chat"
	defaultModel    = "qwen3:4b"
)

// main 是程序入口，负责解析 CLI 参数，并根据启动模式进入 HTTP 代理或命令行对话流程。
func main() {
	options, err := parseCLIOptions(os.Args[1:], os.Stderr)
	if err != nil {
		if err == flag.ErrHelp {
			return
		}
		exitWithError(err, false)
	}

	if options.serveAddr != "" {
		handler := server.NewChatProxyHandler(options.endpoint, options.model, http.DefaultClient)
		if err := newHTTPServer(options.serveAddr, handler).ListenAndServe(); err != nil {
			exitWithError(err, options.debug)
		}
		return
	}

	err = agent.RunChatLoopWithOptions(agent.ChatLoopOptions{
		Endpoint:    options.endpoint,
		Model:       options.model,
		Think:       options.think,
		Client:      http.DefaultClient,
		InitialArgs: options.initialArgs,
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Trace:       tracing.NewTraceHooks(tracing.NewTraceLogger(options.trace, os.Stderr)),
		Debug:       options.debug,
		Mode:        options.mode,
	})
	if err != nil {
		exitWithError(err, options.debug)
	}
}

// parseCLIOptions 解析命令行参数，并把 flag 与环境变量合并成运行时需要的配置。
func parseCLIOptions(args []string, output io.Writer) (cliOptions, error) {
	flags := newCLIFlagSet(output)
	endpoint := flags.String("url", getenvDefault("LOCAL_MODEL_CHAT_URL", defaultEndpoint), "chat API URL")
	model := flags.String("model", getenvDefault("LOCAL_MODEL_NAME", defaultModel), "local model name")
	think := flags.Bool("think", true, "hide model thinking output when true; show it when false")
	trace := flags.Bool("trace", false, "write full agent trace logs to stderr")
	debug := flags.Bool("debug", false, "write structured debug error details to stderr")
	mode := flags.String("mode", string(agent.ModeChat), "agent mode: chat, plan, or strict-plan")
	serveAddr := flags.String("serve", "", "serve a streaming HTTP proxy on this address, for example 127.0.0.1:8080")
	if err := flags.Parse(args); err != nil {
		return cliOptions{}, err
	}
	return cliOptions{
		endpoint:    *endpoint,
		model:       *model,
		think:       *think,
		trace:       *trace,
		debug:       *debug,
		mode:        agent.Mode(*mode),
		serveAddr:   *serveAddr,
		initialArgs: flags.Args(),
	}, nil
}

// newCLIFlagSet 创建 CLI 参数解析器，并统一定制帮助信息的输出格式。
func newCLIFlagSet(output io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet("mini-agent-runtime", flag.ContinueOnError)
	if output == nil {
		output = io.Discard
	}
	flags.SetOutput(output)
	flags.Usage = func() {
		fmt.Fprintf(output, "Usage: %s [options] [message]\n\nOptions:\n", flags.Name())
		flags.VisitAll(func(flagValue *flag.Flag) {
			fmt.Fprintf(output, "  --%-10s %s", flagValue.Name, flagValue.Usage)
			if flagValue.DefValue != "" {
				fmt.Fprintf(output, " (default %s)", flagValue.DefValue)
			}
			_, _ = fmt.Fprintln(output)
		})
	}
	return flags
}

// newHTTPServer 创建带超时保护的 HTTP server，避免 proxy 模式使用 Go 默认 server 暴露连接悬挂风险。
func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      10 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
}

// getenvDefault 读取环境变量，当环境变量为空时返回传入的默认值。
func getenvDefault(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

// exitWithError 统一处理入口层致命错误的日志输出和进程退出。
func exitWithError(err error, debug bool) {
	// main 是程序最外层，只负责把致命错误交给统一 reporter。
	// 真正的发生节点应该由内部包 Wrap 标注，main 这里只代表错误传播到了入口。
	reporter := apperrors.NewReporter(debug, os.Stderr)
	reporter.Log(err)
	reporter.Debug(err)
	os.Exit(1)
}
