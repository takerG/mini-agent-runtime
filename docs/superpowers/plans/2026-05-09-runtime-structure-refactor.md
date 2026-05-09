# Runtime Structure Refactor Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor the agent runtime so model calls, runtime dependencies, and mode orchestration are easier to reuse and extend.

**Architecture:** Introduce a shared `internal/model.Client` for all LLM calls and trace logging. Introduce an `agent.Runtime` to hold shared dependencies and reduce long parameter lists. Keep the existing Ollama protocol package, but split protocol types, request construction, and stream parsing into focused files.

**Tech Stack:** Go 1.22, standard `net/http`, existing Ollama-compatible protocol structs, existing trace/errors/tools packages.

---

## Chunk 1: Shared Model Client

### Task 1: Add `internal/model.Client`

**Files:**
- Create: `internal/model/client.go`
- Create: `internal/model/client_test.go`
- Modify: `internal/errors/errors.go`

- [x] Write a failing test proving `Client.Chat` sends one Ollama request, streams the response, returns content/tool calls, and emits full request/response trace events.
- [x] Run `go test ./internal/model` and verify it fails because the package does not exist.
- [x] Implement `Client`, `ChatOptions`, and `ChatResult`.
- [x] Add `NodeModelClient` or reuse the closest model request node for wrapped errors.
- [x] Run `go test ./internal/model` and verify it passes.

## Chunk 2: Runtime Dependency Object

### Task 2: Add `agent.Runtime`

**Files:**
- Modify: `internal/agent/cli.go`
- Modify: `internal/agent/cli_test.go`
- Modify: `internal/agent/cli_trace_test.go`

- [x] Write a failing test or adapt an existing test to use a runtime-style helper that owns model client, tool registry, reporter, trace, and stdout.
- [x] Run targeted agent tests and verify the failure.
- [x] Implement `Runtime` and move planner/executor turn orchestration onto methods.
- [x] Replace `runPlannerExecutorTurn(...)` long parameter list with `runtime.RunPlannerExecutorTurn(ctx, userMessage)`.
- [x] Run targeted agent tests and verify they pass.

## Chunk 3: Migrate Planner and Executor

### Task 3: Use `model.Client` in planner and executor

**Files:**
- Modify: `internal/planner/planner.go`
- Modify: `internal/planner/planner_test.go`
- Modify: `internal/executor/executor.go`
- Modify: `internal/executor/executor_test.go`

- [x] Write failing tests for new `PlannerOptions` and executor model-client injection.
- [x] Run targeted planner/executor tests and verify failure.
- [x] Replace `Planner.WithTrace` with `NewPlanner(PlannerOptions)`.
- [x] Replace executor direct HTTP/Ollama request logic with `model.Client.Chat`.
- [x] Run targeted tests and verify they pass.

## Chunk 4: Clean API Names and File Boundaries

### Task 4: Split `ollama` package files

**Files:**
- Create: `internal/ollama/types.go`
- Create: `internal/ollama/request.go`
- Keep/modify: `internal/ollama/stream.go`
- Modify: `internal/ollama/stream_test.go`

- [x] Move protocol structs into `types.go`.
- [x] Move request constructors and `NewChatPayload` into `request.go`.
- [x] Keep stream parsing in `stream.go`.
- [x] Prefer options-style constructors for new code; keep existing wrappers only for compatibility.
- [x] Run `go test ./internal/ollama`.

## Chunk 5: Documentation and Verification

### Task 5: Update docs and verify

**Files:**
- Modify: `README.md`

- [x] Update architecture docs to mention `internal/model` and `agent.Runtime`.
- [x] Run `go test -count=1 ./...`.
- [x] Run `go build -buildvcs=false ./...`.
- [x] Convert project text files to CRLF and verify `bareLF=0`.
