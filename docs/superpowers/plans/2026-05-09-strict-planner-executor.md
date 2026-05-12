# Strict Planner Executor Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a strict planner/executor mode where Go executes planner-produced tool calls directly and the model only summarizes final observations.

**Architecture:** Keep existing `--mode plan` as hybrid planner/executor. Add `--mode strict-plan`, executable plan structs/parsing in `internal/planner`, and `Runtime.RunStrictPlannerExecutorTurn` in `internal/agent`. Reuse `ToolRegistry`, `model.Client`, trace hooks, and existing process output format.

**Tech Stack:** Go 1.22, standard library JSON parsing, existing internal model/planner/tools/trace packages.

---

## Chunk 1: Executable Plan Schema

### Task 1: Add executable plan parsing

**Files:**
- Create: `internal/planner/executable_plan.go`
- Create: `internal/planner/executable_plan_test.go`

- [x] Write failing tests for parsing executable JSON with `tool_name` and `arguments`.
- [x] Implement `ExecutablePlan`, `ExecutableStep`, and `ParseExecutablePlan`.
- [x] Run `go test ./internal/planner`.

## Chunk 2: Strict Runtime

### Task 2: Add strict planner/executor runtime path

**Files:**
- Modify: `internal/agent/runtime.go`
- Modify: `internal/agent/cli_test.go`

- [x] Write failing tests proving strict mode executes Go tools directly and sends no tools to the final summary model request.
- [x] Implement `RunStrictPlannerExecutorTurn`.
- [x] Print `[plan]`, `[observation]`, and `Agent:` before final summary content.
- [x] Feed tool errors into observations instead of exiting.
- [x] Run `go test ./internal/agent`.

## Chunk 3: CLI and Docs

### Task 3: Wire CLI mode and documentation

**Files:**
- Modify: `internal/agent/cli.go`
- Modify: `README.md`

- [x] Add `ModeStrictPlan`.
- [x] Route `--mode strict-plan` to strict runtime.
- [x] Document the contrast between `chat`, `plan`, and `strict-plan`.
- [x] Run `go test -count=1 ./...`.
- [x] Run `go build -buildvcs=false ./...`.
- [x] Verify CRLF with `bareLF=0`.
