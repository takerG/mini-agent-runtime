# Architecture Reference

本项目目标是从 0 学习 agent runtime，因此架构要保持可读、可维护、可扩展，同时避免过早复杂化。

## Package Ownership

- `main.go`：唯一入口，只做 CLI 参数解析、模式选择、trace sink 装配和启动。
- `internal/agent`：多轮 CLI loop、session、mode runner、turn coordinator、runtime 编排、memory 注入、lifecycle 装配和工具错误反馈。
- `internal/lifecycle`：run、step、observation、result 的生命周期模型与 recorder。
- `internal/model`：模型调用抽象、Ollama client 适配、请求/响应 trace 捕获。
- `internal/ollama`：Ollama 兼容协议类型、请求 payload、流式 NDJSON 解析。
- `internal/tools`：`Tool` interface、`ToolRegistry`、`ExecutionPolicy`、内置工具、参数解析和校验。
- `internal/planner`：普通计划和 strict executable plan 的结构、解析、格式化。
- `internal/executor`：Hybrid executor 和 strict executor，负责执行计划、收集 observations、生成最终回答；只消费 runtime 注入的共享依赖，不创建默认 registry、trace、reporter 或 tool policy。
- `internal/memory`：memory scope、providers、manager、窗口记忆、摘要记忆、模型摘要器、本地 DB session state。
- `internal/prompts`：系统 prompt 和 prompt 构造函数。
- `internal/trace`：trace event、run/step context、hooks、multi sink、stderr logger、JSONL logger。
- `internal/errors`：错误码、节点、调用链、模型友好格式、debug/operator 输出。
- `internal/server`：HTTP streaming proxy。

## Extension Rules

- 新增 CLI 行为：优先改 `main.go` 的参数装配和 `internal/agent` 的依赖结构，不把业务逻辑放进 `main.go`。
- 新增工具：新增 `internal/tools/<tool_name>.go`，实现 `Tool`，注册到默认 registry，补测试。
- 新增工具治理策略：优先扩展 `internal/tools.ExecutionPolicy`，再通过 `ChatLoopDependencies` 或 runtime options 注入。
- 调整 executor 依赖：优先扩展 `executor.Dependencies`，由 `Runtime` 或 CLI 注入，不在 `NewExecutor` / `NewStrictExecutor` 中补 runtime 层默认值。
- 新增 lifecycle 节点：优先使用 `internal/lifecycle.Recorder` 记录 run/step/observation，再让 trace 复用同一组 run/step ID。
- 新增 trace 输出：优先实现 `trace.TraceSink`，通过 `trace.NewMultiSink` 组合，不改业务流程。
- 新增 memory 策略：在 `internal/memory` 新增 provider 或 summarizer，通过 manager 组合，不增加用户启动参数。
- 调整单轮执行流程：保持 `internal/agent/turn.go` 作为统一收口，确保 CLI runner 和 direct runtime API 都经过同一套 run lifecycle 与 memory 写入；具体模式执行必须通过具名 executor 实现，不用匿名函数隐藏真实调用。
- 新增 planner/executor 能力：优先放在 `internal/planner` 和 `internal/executor`，让 `internal/agent` 只做编排。
- 新增 prompt：放入 `internal/prompts`，避免 prompt 文本散落在 runtime 逻辑里。
- 新增错误类型：通过 `internal/errors` 扩展错误码和节点，保持模型友好错误格式稳定。
