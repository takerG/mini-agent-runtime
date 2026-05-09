package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"mini-agent-runtime/internal/agent"
	apperrors "mini-agent-runtime/internal/errors"
	"mini-agent-runtime/internal/server"
	tracing "mini-agent-runtime/internal/trace"
)

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
		if err := http.ListenAndServe(options.serveAddr, handler); err != nil {
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

func parseCLIOptions(args []string, output io.Writer) (cliOptions, error) {
	flags := newCLIFlagSet(output)
	endpoint := flags.String("url", getenvDefault("LOCAL_MODEL_CHAT_URL", defaultEndpoint), "chat API URL")
	model := flags.String("model", getenvDefault("LOCAL_MODEL_NAME", defaultModel), "local model name")
	think := flags.Bool("think", true, "hide model thinking output when true; show it when false")
	trace := flags.Bool("trace", false, "write full agent trace logs to stderr")
	debug := flags.Bool("debug", false, "write structured debug error details to stderr")
	mode := flags.String("mode", string(agent.ModeChat), "agent mode: chat or plan")
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
			fmt.Fprintln(output)
		})
	}
	return flags
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
