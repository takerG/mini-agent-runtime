# mini-agent-runtime

A minimal Go agent runtime for streaming multi-turn chat from a local Ollama-compatible model API.

## Usage

Start an interactive multi-turn chat:

```powershell
go run . --model llama3.2
```

Use `go run .` from the project directory. The root package now keeps only `main.go`; all agent components live under `internal/`.

Then type one message per line. Use `/exit`, `/quit`, `exit`, or `quit` to leave.

You can also pass the first message as command arguments. After the first response, the CLI continues reading follow-up messages:

```powershell
go run . --model llama3.2 "介绍一下你自己"
```

Defaults:

- API URL: `http://localhost:11434/api/chat`
- Model: `qwen3:4b`
- Think output: hidden by default with `--think=true`

You can override defaults with flags:

```powershell
go run . --url http://localhost:11434/api/chat --model qwen2.5 "你好"
```

For thinking models such as Qwen3, use `--think=false` when you want to show the think stream:

```powershell
go run . --model qwen3:4b --think=false
```

Enable trace logs when you want to see what the agent is doing at each step:

```powershell
go run . --model qwen3:4b --trace
```

Trace logs are written to stderr and include user turns, full model request payloads, full model response content/tool calls, tool calls, tool results, and final answers. This is useful for debugging and interview demos.

Internally, trace uses structured hooks. The default CLI sink formats those events as stderr logs, and future sinks can reuse the same events for files, metrics, or UI panels.

Enable debug error output when you want structured error details on stderr:

```powershell
go run . --model qwen3:4b --debug "23 / 0 等于多少？"
```

Debug output is controlled by `--debug`. It includes stable error codes, the origin node where the error happened, and the node chain that shows how the error traveled through the runtime.

Enable planner/executor mode when you want the runtime to plan first, then execute the plan with native tool calls:

```powershell
go run . --mode plan "现在几点？顺便算一下 23 * 19"
```

In `plan` mode the CLI prints the full process:

```text
[plan]
1. tool_call calculator {"a":23,"b":19,"op":"*"}
2. tool_call current_time {}

[observation]
1. calculator -> 437
2. current_time -> 2026-05-08 20:51:42 CST

Agent:
计算结果是 437，当前时间是 2026-05-08 20:51:42 CST。
```

The default mode is `chat`, which preserves the original direct native tool-calling behavior.

## Agent Tools

The CLI sends two tool definitions to the model on every chat turn:

- `current_time`: returns the current local time.
- `calculator`: runs basic `+`, `-`, `*`, `/` calculations.

When the model decides a question needs a tool, the app executes the Go function, appends the tool result to the conversation history, and asks the model again for the final answer.

If a tool does not exist or returns an error, the CLI does not exit immediately. It appends a `role=tool` message such as `tool error: ...` and lets the model decide whether to retry, switch tools, or explain the problem to the user.

Tools are managed through `ToolRegistry` in `internal/tools`. To add a new tool, implement the `Tool` interface with `Name()`, `Description()`, `Definition()`, and `Execute(ctx, args)`, then register the tool instance in `NewDefaultToolRegistry`.

Example prompts:

```text
现在几点？
23 * 19 等于多少？
```

Or with environment variables:

```powershell
$env:LOCAL_MODEL_CHAT_URL = "http://localhost:11434/api/chat"
$env:LOCAL_MODEL_NAME = "qwen2.5"
go run . "你好"
```

## Streaming HTTP Proxy

Start a small HTTP server that streams model chunks to the client as soon as they arrive:

```powershell
go run . --serve 127.0.0.1:8080 --model qwen2.5
```

Call it with JSON:

```powershell
curl.exe -N -X POST http://127.0.0.1:8080/chat `
  -H "Content-Type: application/json" `
  -d "{\"message\":\"你好\"}"
```

The server posts to `http://localhost:11434/api/chat` with `stream: true`, reads each streamed JSON line, writes `message.content` to the HTTP response, and flushes every chunk immediately.

## Architecture

The entrypoint is intentionally thin:

- `main.go`: parses flags, chooses CLI or HTTP server mode, wires components together.
- `internal/agent`: owns the multi-turn CLI loop, runtime dependency wiring, tool-call rounds, history, and tool error feedback.
- `internal/model`: owns shared model invocation, request/response trace events, HTTP status handling, and stream capture.
- `internal/planner`: asks the model for a structured JSON plan before execution.
- `internal/executor`: follows a plan, reuses native tool calls, and streams the final answer.
- `internal/ollama`: owns Ollama-compatible protocol types, request payload construction, and streaming NDJSON parsing.
- `internal/tools`: owns `Tool`, `ToolRegistry`, built-in tools, and tool argument validation.
- `internal/trace`: owns structured trace events, hooks, and the stderr logger sink.
- `internal/errors`: owns error codes, runtime nodes, model-friendly error formatting, operator logs, and debug output.
- `internal/server`: owns the streaming HTTP proxy handler.

This layout keeps provider protocol, tool execution, tracing, runtime orchestration, and transport concerns separated, so later tools or model providers can be added without growing `main.go`.

## Development

```powershell
$env:GOCACHE = "D:\work\mini-agent-runtime\.gocache"
go test ./...
go build -buildvcs=false ./...
```
