package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
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
	defaultModel = "llama3.2"
)

func main() {
	// flag 包是 Go 标准库里的命令行参数解析工具。
	// 这里把“模型地址”“模型名称”“是否启动 HTTP 服务”都做成可配置项，
	// 这样第一版虽然很小，但已经具备后续扩展 agent runtime 的基本入口。
	endpoint := flag.String("url", getenvDefault("LOCAL_MODEL_CHAT_URL", defaultEndpoint), "chat API URL")
	model := flag.String("model", getenvDefault("LOCAL_MODEL_NAME", defaultModel), "local model name")
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

	// 不启动 HTTP 服务时，程序就是一个最小 CLI：
	// 用户可以把一句话直接写在命令后面，也可以运行后再从 stdin 输入一行。
	userMessage, err := readUserMessage(flag.Args(), os.Stdin)
	if err != nil {
		exitWithError(err)
	}

	// 构造发给本地模型的 HTTP 请求。这里不会一次性等模型完整回答，
	// 请求体里会设置 stream=true，让模型服务按流式响应逐段返回。
	req, err := NewChatRequest(*endpoint, *model, userMessage)
	if err != nil {
		exitWithError(err)
	}

	// 发送 HTTP 请求后，resp.Body 是一个流。
	// 只要服务端还在生成内容，这个 Body 就可能不断读到新的 JSON 行。
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		exitWithError(fmt.Errorf("post chat request: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		exitWithError(fmt.Errorf("chat request failed: %s", resp.Status))
	}

	// 核心流式处理：
	// StreamChatContent 会从 resp.Body 一行一行读取模型响应，
	// 每读到 response.message.content 就立即写到 os.Stdout。
	// 这就是“流式接收模型响应，同时流式对外输出”的 CLI 版本。
	if err := StreamChatContent(resp.Body, os.Stdout); err != nil {
		exitWithError(err)
	}
	fmt.Fprintln(os.Stdout)
}

func readUserMessage(args []string, stdin io.Reader) (string, error) {
	// 优先使用命令行参数，这样可以：
	//   go run . "你好"
	// 如果用户传了多个单词，strings.Join 会把它们拼成一句话。
	if len(args) > 0 {
		message := strings.TrimSpace(strings.Join(args, " "))
		if message != "" {
			return message, nil
		}
	}

	// 如果命令行没有给消息，就提示用户输入一行。
	// Scanner 默认按行读取，因此用户按回车后，这一轮输入就结束。
	fmt.Fprint(os.Stderr, "You: ")
	scanner := bufio.NewScanner(stdin)
	if scanner.Scan() {
		message := strings.TrimSpace(scanner.Text())
		if message == "" {
			return "", fmt.Errorf("empty message")
		}
		return message, nil
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read message: %w", err)
	}
	return "", fmt.Errorf("no message provided")
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
