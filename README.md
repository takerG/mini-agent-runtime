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

Write machine-readable JSONL trace events when you want to replay or inspect the whole run lifecycle:

```powershell
go run . --model qwen3:4b --trace-jsonl trace.jsonl "现在几点？"
```

Each JSONL event includes a timestamp, event name, optional `run_id`, optional `step_id`, optional `parent_step_id`, and structured `data`.

Enable debug error output when you want structured error details on stderr:

```powershell
go run . --model qwen3:4b --debug "23 / 0 等于多少？"
```

Debug output is controlled by `--debug`. It includes stable error codes, the origin node where the error happened, and the node chain that shows how the error traveled through the runtime.

Enable hybrid planner/executor mode when you want the runtime to plan first, then let the executor model continue native tool calling:

```powershell
go run . --mode plan "现在几点？顺便算一下 23 * 19"
```

In `plan` mode, the planner creates a task plan, then the executor model decides native `tool_calls`. The CLI prints the full process:

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

Enable strict planner/executor mode when you want the planner to output executable JSON and Go to execute each tool call directly:

```powershell
go run . --mode strict-plan "请先计算 23 * 19，再获取当前时间，最后生成一个简短记录。"
```

In `strict-plan` mode, the model only chooses the executable plan and writes the final summary. Go parses `tool_name` and `arguments`, executes tools through `ToolRegistry`, collects observations, and sends those observations to the model without exposing tools on the summary request.

Mode comparison:

- `chat`: the model directly uses native tool calling during the conversation.
- `plan`: hybrid planner/executor; planner writes a plan, executor model still decides native tool calls.
- `strict-plan`: strict planner/executor; planner writes executable JSON, Go executes tools, model only summarizes observations.

The default mode is `chat`, which preserves the original direct native tool-calling behavior.

## Agent Runtime Lifecycle

Every user turn is now recorded as a `Run` with stable `run_id`, mode, input, status, timing, steps, observations, and final result. Model requests, planner calls, executor work, tool calls, observations, and summaries are represented as lifecycle steps.

The lifecycle model is implemented in `internal/lifecycle` and is shared by `chat`, `plan`, and `strict-plan`. Trace events reuse the same run/step context, so stderr trace logs and JSONL trace files can be correlated step by step.

Single-turn lifecycle coordination is centralized in `internal/agent/turn.go`. CLI runners and direct runtime APIs both use this path, so final answers are consistently traced and written back to memory across `chat`, `plan`, and `strict-plan`.

## Agent Tools

The CLI sends two tool definitions to the model on every chat turn:

- `current_time`: returns the current local time.
- `calculator`: runs basic `+`, `-`, `*`, `/` calculations.

When the model decides a question needs a tool, the app executes the Go function, appends the tool result to the conversation history, and asks the model again for the final answer.

If a tool does not exist or returns an error, the CLI does not exit immediately. It appends a `role=tool` message such as `tool error: ...` and lets the model decide whether to retry, switch tools, or explain the problem to the user.

Tools are managed through `ToolRegistry` in `internal/tools`. To add a new tool, implement the `Tool` interface with `Name()`, `Description()`, `Definition()`, and `Execute(ctx, args)`, then register the tool instance in `NewDefaultToolRegistry`.

Tool execution can be governed by `ExecutionPolicy`, including timeout, retry, and allow/deny checks. The default policy is still simple: one attempt, no timeout, no retry.

The CLI wiring also supports dependency injection through `ChatLoopDependencies`, so tests or future runtimes can provide a custom `ToolRegistry`, tool policy, memory manager, lifecycle factory, trace hooks, or error reporter without changing the CLI loop.

Runtime and CLI are the composition roots for shared dependencies. `internal/executor` receives those dependencies through `executor.Dependencies` and does not create default tool registries, trace hooks, reporters, or tool policies by itself.

## Agent Memory

The runtime includes a composable memory layer in `internal/memory`. Memory is controlled by code, not CLI flags, so different agent deployments can switch or combine memory strategies without changing the user-facing interface.

Memory read/write is part of the unified turn lifecycle. Planner modes read memory before planner/executor requests, and direct runtime calls also commit the final user/assistant turn back into memory when the turn succeeds.

The first version includes:

- `WindowMemory`: session memory for the most recent N completed turns.
- `SummaryMemory`: user or session memory that maintains a rolling summary. The default summarizer is local and string-based for offline demos, and `NewModelSummarizer` / `NewModelSummaryManager` can use the configured model client to generate real summaries.
- `DBSessionStateMemory`: session state access functions backed by an in-memory map for now. It models future DB access without calling an external database.

`MemoryManager` composes providers and exposes one `Context` read path plus one `AppendTurn` write path. The same manager is used by `chat`, `plan`, and `strict-plan` modes. Memory context is injected as a system message only when a provider has data.

User memory and session memory are separated by `memory.Scope`:

- `ScopeUser`: durable user-level facts that can be shared across sessions.
- `ScopeSession`: current-session context, recent turns, and session state.

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
- `internal/agent`: owns the multi-turn CLI loop, `Session` history/memory state, mode runners, runtime dependency wiring, and tool error feedback.
- `internal/model`: owns shared model invocation, request/response trace events, HTTP status handling, and stream capture.
- `internal/lifecycle`: owns run, step, observation, result records, and lifecycle recording helpers.
- `internal/memory`: owns memory providers, user/session scope separation, memory composition, and local simulated session state.
- `internal/prompts`: owns reusable planner, executor, strict planner, and responder prompt templates.
- `internal/planner`: asks the model for a structured JSON plan before execution.
- `internal/executor`: follows hybrid plans with native tool calls, directly executes strict executable plans, and streams the final answer.
- `internal/ollama`: owns Ollama-compatible protocol types, request payload construction, and streaming NDJSON parsing.
- `internal/tools`: owns `Tool`, `ToolRegistry`, execution policy, built-in tools, and tool argument validation.
- `internal/trace`: owns structured trace events, hooks, run/step context, stderr sink, multi sink, and JSONL sink.
- `internal/errors`: owns error codes, runtime nodes, model-friendly error formatting, operator logs, and debug output.
- `internal/server`: owns the streaming HTTP proxy handler.

This layout keeps provider protocol, tool execution, tracing, runtime orchestration, and transport concerns separated, so later tools or model providers can be added without growing `main.go`.

Default dependency assembly happens in `main.go`, `internal/agent`, and `Runtime`. Lower-level executors consume dependencies instead of creating their own runtime defaults, which keeps ownership clear and avoids hidden behavior differences between modes.

## Development

Codex-facing project instructions start from `AGENTS.md`, which is the auto-discovered root instruction file. Detailed reusable guidance lives in `.codex/skills/mini-agent-runtime/SKILL.md` and its `references/` directory. `docs/specs/project-constraints.md` mirrors the canonical file layout for human maintainers.

The project-level code quality spec lives in `CODING_SPEC.md`.

Run the full local quality gate before committing code:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/check.ps1
```

If `make` is available:

```powershell
make check
```

`scripts/check.ps1` runs gofmt check, CRLF check, `go vet ./...`, `go test ./...`, `staticcheck ./...`, and `golangci-lint run`. Install missing optional tools with:

```powershell
go install honnef.co/go/tools/cmd/staticcheck@latest
```

For `golangci-lint`, follow the official installation guide: https://golangci-lint.run/welcome/install/

Minimum fallback checks:

```powershell
$env:GOCACHE = Join-Path (Get-Location) ".gocache"
go test -count=1 ./...
go build -buildvcs=false ./...
```
