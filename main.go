package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
)

const (
	// 本地模型服务的默认地址。
	// 这里使用的是 Ollama 的 chat API 地址。如果你后续换成别的本地模型运行时，
	// 只要它提供兼容的 HTTP 接口，就可以通过 -url 或环境变量覆盖这个值。
	defaultEndpoint = "http://localhost:11434/api/chat"

	// 默认模型名只是一个示例。不同电脑上安装的模型可能不同，
	// 所以 main 函数里也提供了 -model 参数和 LOCAL_MODEL_NAME 环境变量。
	defaultModel = "qwen3:4b"
)

func main() {
	// flag 包是 Go 标准库里的命令行参数解析工具。
	// 这里把“模型地址”“模型名称”“是否启动 HTTP 服务”都做成可配置项，
	// 这样第一版虽然很小，但已经具备后续扩展 agent runtime 的基本入口。
	endpoint := flag.String("url", getenvDefault("LOCAL_MODEL_CHAT_URL", defaultEndpoint), "chat API URL")
	model := flag.String("model", getenvDefault("LOCAL_MODEL_NAME", defaultModel), "local model name")
	think := flag.Bool("think", true, "hide model thinking output when true; show it when false")
	trace := flag.Bool("trace", false, "write agent trace logs to stderr")
	serveAddr := flag.String("serve", "", "serve a streaming HTTP proxy on this address, for example 127.0.0.1:8080")
	flag.Parse()

	// 如果传入 -serve，就不再作为一次性 CLI 运行，而是启动一个 HTTP 代理服务。
	// 这个服务接收外部请求，再去调用本地模型，并把模型返回的 token/chunk
	// 立刻转发给调用方。做 agent 时，这种“边接收边输出”的模式很重要，
	// 因为用户可以更早看到模型开始思考/回答，整体体感速度会更快。
	if *serveAddr != "" {
		handler := NewChatProxyHandler(*endpoint, *model, http.DefaultClient)
		if err := http.ListenAndServe(*serveAddr, handler); err != nil {
			exitWithError(err)
		}
		return
	}

	// 不启动 HTTP 服务时，程序进入多轮 CLI 对话模式。
	// flag.Args() 如果有内容，会作为第一轮用户输入；第一轮结束后继续读取 stdin，
	// 直到用户输入 /exit、/quit、exit、quit，或 stdin 到达 EOF。
	if err := RunChatLoopWithTrace(*endpoint, *model, *think, http.DefaultClient, flag.Args(), os.Stdin, os.Stdout, os.Stderr, NewTraceHooks(NewTraceLogger(*trace, os.Stderr))); err != nil {
		exitWithError(err)
	}
}

func getenvDefault(name string, fallback string) string {
	// 读取环境变量，并把空字符串视为“没有设置”。
	// 这样用户可以临时覆盖默认配置，而不需要改源码。
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func exitWithError(err error) {
	// 统一错误出口：错误写到 stderr，正常模型输出写到 stdout。
	// 这种区分有利于以后把模型输出通过管道交给其他程序处理。
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
