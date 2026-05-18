package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"mini-agent-runtime/internal/agent"
	apperrors "mini-agent-runtime/internal/errors"
	evalrunner "mini-agent-runtime/internal/eval"
	"mini-agent-runtime/internal/server"
	tracing "mini-agent-runtime/internal/trace"
)

// cliOptions 保存入口层解析后的 CLI 配置。
type cliOptions struct {
	endpoint    string
	model       string
	think       bool
	trace       bool
	traceJSONL  string
	debug       bool
	mode        agent.Mode
	serveAddr   string
	evalPath    string
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
		if errors.Is(err, flag.ErrHelp) {
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

	if options.evalPath != "" {
		if err := runEval(options); err != nil {
			exitWithError(err, options.debug)
		}
		return
	}

	traceHooks, traceCloser, err := buildTraceHooks(options, os.Stderr)
	if err != nil {
		exitWithError(err, options.debug)
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
		Trace:       traceHooks,
		Debug:       options.debug,
		Mode:        options.mode,
	})
	if closeErr := traceCloser.Close(); closeErr != nil && err == nil {
		err = fmt.Errorf("close trace sink: %w", closeErr)
	}
	if err != nil {
		exitWithError(err, options.debug)
	}
}

// parseCLIOptions 解析命令行参数，并把 flag 与环境变量合并成运行时需要的配置。
func parseCLIOptions(args []string, output io.Writer) (cliOptions, error) {
	flags := newCLIFlagSet(output)
	endpoint := flags.String("url", getENVDefault("LOCAL_MODEL_CHAT_URL", defaultEndpoint), "chat API URL")
	model := flags.String("model", getENVDefault("LOCAL_MODEL_NAME", defaultModel), "local model name")
	think := flags.Bool("think", true, "hide model thinking output when true; show it when false")
	trace := flags.Bool("trace", false, "write full agent trace logs to stderr")
	traceJSONL := flags.String("trace-jsonl", "", "write structured JSONL trace events to this file")
	debug := flags.Bool("debug", false, "write structured debug error details to stderr")
	mode := flags.String("mode", string(agent.ModeChat), "agent mode: chat, plan, or strict-plan")
	serveAddr := flags.String("serve", "", "serve a streaming HTTP proxy on this address, for example 127.0.0.1:8080")
	evalPath := flags.String("eval", "", "run eval suite from JSON file, for example docs/evals/basic.json")
	if err := flags.Parse(args); err != nil {
		return cliOptions{}, err
	}
	if *evalPath != "" && *serveAddr != "" {
		return cliOptions{}, fmt.Errorf("--eval cannot be used with --serve")
	}
	return cliOptions{
		endpoint:    *endpoint,
		model:       *model,
		think:       *think,
		trace:       *trace,
		traceJSONL:  *traceJSONL,
		debug:       *debug,
		mode:        agent.Mode(*mode),
		serveAddr:   *serveAddr,
		evalPath:    *evalPath,
		initialArgs: flags.Args(),
	}, nil
}

// runEval 读取 eval suite 并运行所有用例，失败用例会转换为 CLI 错误。
func runEval(options cliOptions) error {
	suite, err := evalrunner.LoadSuite(options.evalPath)
	if err != nil {
		return err
	}
	traceSinks, traceCloser, err := buildTraceSinks(options, os.Stderr)
	if err != nil {
		return err
	}
	report, err := evalrunner.NewRunner(evalrunner.RunnerOptions{
		Endpoint:   options.endpoint,
		Model:      options.model,
		Think:      options.think,
		Client:     http.DefaultClient,
		Output:     os.Stderr,
		TraceSinks: traceSinks,
	}).RunSuite(context.Background(), suite)
	closeErr := traceCloser.Close()
	if err != nil {
		return err
	}
	report.WriteText(os.Stdout)
	if closeErr != nil {
		return fmt.Errorf("close trace sink: %w", closeErr)
	}
	return report.FailureError()
}

// buildTraceHooks 根据 CLI 配置创建 trace hooks，并在需要时打开 JSONL 文件 sink。
func buildTraceHooks(options cliOptions, stderr io.Writer) (*tracing.TraceHooks, io.Closer, error) {
	sinks, closer, err := buildTraceSinks(options, stderr)
	if err != nil {
		return nil, nil, err
	}
	return tracing.NewTraceHooks(tracing.NewMultiSink(sinks...)), closer, nil
}

// buildTraceSinks 根据 CLI 配置创建可复用的 trace sink 列表。
func buildTraceSinks(options cliOptions, stderr io.Writer) ([]tracing.TraceSink, io.Closer, error) {
	sinks := []tracing.TraceSink{
		tracing.NewTraceLogger(options.trace, stderr),
	}
	if options.traceJSONL == "" {
		return sinks, closeFunc(func() error { return nil }), nil
	}
	file, err := os.Create(options.traceJSONL)
	if err != nil {
		return nil, nil, fmt.Errorf("create trace jsonl file: %w", err)
	}
	sinks = append(sinks, tracing.NewTraceJSONLLogger(true, file))
	return sinks, file, nil
}

type closeFunc func() error

// Close 执行轻量 close 回调，用于统一返回 io.Closer。
func (f closeFunc) Close() error {
	return f()
}

// newCLIFlagSet 创建 CLI 参数解析器，并统一定制帮助信息的输出格式。
func newCLIFlagSet(output io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet("mini-agent-runtime", flag.ContinueOnError)
	if output == nil {
		output = io.Discard
	}
	flags.SetOutput(output)
	flags.Usage = func() {
		_, _ = fmt.Fprintf(output, "Usage: %s [options] [message]\n\nOptions:\n", flags.Name())
		flags.VisitAll(func(flagValue *flag.Flag) {
			_, _ = fmt.Fprintf(output, "  --%-10s %s", flagValue.Name, flagValue.Usage)
			if flagValue.DefValue != "" {
				_, _ = fmt.Fprintf(output, " (default %s)", flagValue.DefValue)
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

// getENVDefault 读取环境变量，当环境变量为空时返回传入的默认值。
func getENVDefault(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

// exitWithError 统一处理入口层致命错误的日志输出和进程退出。
func exitWithError(err error, debug bool) {
	// main 是程序最外层，只负责把致命错误交给统一 reporter。
	// 真正的发生节点应由内部包 Wrap 标注，main 这里只代表错误传播到了入口。
	reporter := apperrors.NewReporter(debug, os.Stderr)
	reporter.Log(err)
	reporter.Debug(err)
	os.Exit(1)
}
