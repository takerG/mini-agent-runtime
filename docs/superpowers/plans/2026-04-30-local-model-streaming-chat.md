# Local Model Streaming Chat Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a minimal Go CLI that reads one user message, streams chat output from a local Ollama-compatible endpoint, and prints `response.message.content`.

**Architecture:** Keep the first version as a small command-line app. Put request/response streaming behavior behind testable functions so the CLI stays thin and later agent runtime work can reuse the client.

**Tech Stack:** Go standard library only, `net/http`, `encoding/json`, `bufio`, `flag`, `testing`.

---

## Chunk 1: Minimal CLI

### Task 1: Streaming Response Parser

**Files:**
- Create: `go.mod`
- Create: `stream_test.go`
- Create: `stream.go`

- [ ] **Step 1: Write failing parser tests**

Test that newline-delimited streaming JSON emits only `message.content` chunks and skips empty terminal records.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./...`
Expected: FAIL because parser symbols are not implemented yet.

- [ ] **Step 3: Implement minimal parser**

Decode each JSON line into a response struct, write content chunks to the provided writer, and stop cleanly at EOF.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS.

### Task 2: CLI and HTTP Chat Request

**Files:**
- Create: `main.go`
- Modify: `README.md`

- [ ] **Step 1: Add CLI testable boundaries through existing parser**

Keep flags and stdin handling in `main.go`; keep streaming and request types in `stream.go`.

- [ ] **Step 2: Implement POST to local chat endpoint**

POST JSON to `http://localhost:11434/api/chat` by default with `stream: true`.

- [ ] **Step 3: Document usage**

Add `go run . -model <name>` and environment variable examples.

- [ ] **Step 4: Verify**

Run: `go test ./...` and `go build ./...`.
