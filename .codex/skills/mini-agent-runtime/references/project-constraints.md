# Project Constraints

本文件是 mini-agent-runtime 的详细工程约束。`AGENTS.md` 是自动加载入口，本文件是需要展开时读取的长期规范。

## 1. 代码与注释

- MUST 使用 Go 语言实现项目主体。
- MUST 保持根目录只有 `main.go` 一个 Go 入口文件；其他能力放入 `internal/` 包。
- MUST 为所有 Go 函数和方法添加中文 GoDoc 注释，且注释以函数名或方法名开头。
- MUST 后续新增函数时同步新增同风格注释，不等到集中补文档。
- SHOULD 保持代码清晰、简洁、适合学习；避免为了抽象而抽象。
- SHOULD 当函数参数列表过长或职责混杂时，优先抽象为 options/config 结构体或拆分组件。
- SHOULD 避免为简单字段访问创建一次性局部别名；只有变量会被改写、多次复用、命名复杂表达式或明显降低阅读成本时才定义新变量。
- MUST 遵守根目录 `CODING_SPEC.md` 中的 Go 质量、错误处理、lint、测试和提交前自检规范。

## 2. 文件格式

- MUST 默认使用 CRLF 换行。
- MUST 提交前避免出现裸 LF，重点检查 `.go`、`.md`、`.mod`、`.gitignore`、`.gitattributes`、`.editorconfig`、`.yaml`。
- MUST 不提交 `.idea/`、`.gocache/`、可执行文件和本地生成物。
- SHOULD 使用 `gofmt` 格式化 Go 文件。

## 3. CLI 体验

- MUST 使用双横线参数，例如 `--mode`、`--trace`、`--trace-jsonl`、`--debug`、`--model`、`--think`。
- MUST 支持多轮 CLI 对话，不只支持启动参数中的单轮问题。
- MUST 支持启动时传入首轮用户消息，首轮结束后继续进入交互式 CLI。
- MUST 支持 `/exit`、`/quit`、`exit`、`quit` 退出。
- MUST 通过启动参数传入 think 控制，不允许写死。
- MUST 保持当前已验证的 think 语义：`--think=true` 时不显示 thinking，`--think=false` 时显示 thinking。

## 4. 流式输出

- MUST 对本地模型使用真正的 streaming 调用。
- MUST 向 Ollama 兼容 endpoint 发送 `stream: true` 请求。
- MUST 收到上游 chunk 后尽快解析并输出 `response.message.content`，不要等待完整响应结束。
- MUST HTTP proxy 模式下每个 chunk 后 flush。
- SHOULD 避免使用会限制长行的流读取方式；长 thinking、长 content、长 tool arguments 都不能导致流中断。

## 5. Agent 模式

- MUST 保留默认 `chat` 模式，使用模型原生 tool calling 完成普通对话。
- MUST 保留 `plan` 模式作为 Hybrid Planner/Executor：Planner 先给自然语言计划，Executor 再让模型根据计划继续原生 tool calling。
- MUST 保留 `strict-plan` 模式作为 Strict Planner/Executor：Planner 输出可执行 JSON，Go 解析并直接执行工具，模型只负责最终总结。
- MUST plan 类模式下向 CLI 展示完整过程，包括 `[plan]`、`[observation]` 和 `Agent:`。
- MUST strict-plan 模式下避免让模型二次决定工具调用；工具执行权归 Go runtime。
- SHOULD 命名和架构清晰区分 planner、executor、runtime、model client、tools、lifecycle、trace、errors。

## 6. Run Lifecycle

- MUST 每次用户请求记录为一个 `Run`，包含 `run_id`、mode、input、status、时间、steps、observations 和最终 result。
- MUST 模型请求、planner、executor、tool call、summary 等关键节点记录为 step。
- MUST 工具结果、工具错误、模型返回、最终回答等关键数据记录为 observation。
- MUST `chat`、`plan`、`strict-plan` 共用同一套 lifecycle 模型。
- SHOULD trace 事件复用 lifecycle 的 run/step ID，便于 stderr、JSONL、未来 UI 或 replay 对齐。

## 7. Tools 系统

- MUST 使用 `Tool` interface 和 `ToolRegistry` 注册机制管理工具。
- MUST 通过 `registry.Definitions()` 获取模型可见工具定义。
- MUST 默认通过 `registry.Execute(ctx, call)` 执行工具调用；需要治理时通过 `registry.ExecuteWithPolicy(ctx, call, policy)`。
- MUST 禁止在 CLI 或 runtime 中回退到硬编码 `switch tool_name` 的分发方式。
- MUST 每个 tool 使用独立 `.go` 文件，并使用工具名命名，例如 `calculator.go`、`current_time.go`。
- MUST 默认注册内置工具 `calculator` 和 `current_time`。
- MUST 当工具不存在或工具返回错误时，不直接退出 agent 对话，而是把结构化错误作为 observation 交给模型继续处理。
- SHOULD 工具治理能力优先放入 `ExecutionPolicy`，包括 timeout、retry、allow/deny，为 human approval 预留扩展空间。

## 8. Memory 系统

- MUST 区分 user memory 和 session memory。
- MUST memory 策略通过代码装配、切换或组合，不通过 CLI 参数暴露。
- MUST 支持最近 N 轮窗口记忆、摘要 memory、DB session state 的本地内存模拟实现。
- SHOULD DB 或向量存储类能力先实现访问函数和接口边界，首版不访问外部数据库。

## 9. Trace Log

- MUST 使用 hook/sink 形式的 trace 机制，不只依赖业务逻辑里零散手写日志。
- MUST `--trace` 下完整展示每轮模型入参、模型返回内容和工具调用。
- MUST trace 关键 agent 节点，包括用户输入、模型请求、模型响应、工具调用、工具结果、工具错误、planner、executor、最终回答。
- MUST trace 事件支持 run/step/parent_step 上下文。
- SHOULD trace 输出写入 stderr，避免污染 stdout 中的模型流式回答。
- SHOULD 支持 JSONL sink，事件可逐行 JSON.parse，用于复盘、面试展示、自动测试或未来 replay。
- SHOULD 记录隐私风险：trace 可能包含用户输入、工具参数和工具结果；外部工具扩展时考虑脱敏和截断策略。

## 10. 错误处理

- MUST 使用统一 errors 包接管错误处理能力。
- MUST 对可能被 wrap 的错误使用标准库 `errors.Is` / `errors.As` 判断，禁止直接使用 `err == targetErr` 判断错误链。
- MUST 维护稳定的错误码、运行节点和调用链信息。
- MUST 提供面向模型理解的错误格式化能力，便于工具失败后交还给模型处理。
- MUST 提供面向操作者的日志输出能力。
- MUST 通过 `--debug` 控制 debug 信息是否打印到命令行。
- MUST debug/error 日志标注错误真实发生节点和传播链路。

## 11. Prompts

- MUST prompts 使用清晰中文文本定义。
- MUST 多行 prompt 使用 Go 原始字符串形式。
- MUST strict planner prompt 只要求输出合法 JSON，不输出 Markdown、解释或最终回答。
- MUST strict planner 只能使用 runtime 提供的 `tools` 字段，不重复引入 `AvailableTools` 之类字段。

## 12. 架构边界

- MUST `main.go` 只负责参数解析、模式选择、trace sink 装配和依赖启动。
- MUST `internal/agent` 负责 CLI loop、runtime 编排、历史消息和模式分发。
- MUST `internal/lifecycle` 负责 run、step、observation、result 记录。
- MUST `internal/model` 负责共享模型调用、HTTP 请求、trace 和响应捕获。
- MUST `internal/ollama` 负责 Ollama 兼容协议类型、请求构造和流式解析。
- MUST `internal/tools` 负责工具接口、注册表、执行策略、内置工具和参数校验。
- MUST `internal/planner` 负责 planner 输出结构和解析。
- MUST `internal/executor` 负责 Hybrid 和 Strict Planner/Executor 的执行阶段。
- MUST `internal/executor` 只消费 runtime 注入的共享依赖，不创建默认 `ToolRegistry`、`TraceHooks`、`Reporter` 或 `ExecutionPolicy`。
- MUST `internal/memory` 负责 memory provider、scope、组合和本地模拟存储。
- MUST `internal/prompts` 负责系统 prompt 和 prompt 构造。
- MUST `internal/trace` 负责 trace 事件、run/step context、hooks 和 sink。
- MUST `internal/errors` 负责错误码、节点、格式化、日志和 debug 输出。
- MUST `internal/server` 负责 HTTP streaming proxy。

## 13. 验证要求

- MUST 代码变更后优先运行 `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/check.ps1`，或在安装了 `make` 的环境运行 `make check`。
- MUST `scripts/check.ps1` 至少覆盖 `gofmt` 检查、CRLF 检查、`go vet ./...`、`go test ./...`、`staticcheck ./...` 和 `golangci-lint run`。
- MUST 最小验证保留 `go test -count=1 ./...` 和 `go build -buildvcs=false ./...`。
- MUST 文档或换行相关变更后至少运行 `git diff --check`。
- SHOULD 涉及函数新增、重命名或大规模改动后检查函数注释覆盖。
- SHOULD 若默认 Go 全局 build cache 异常，优先使用项目内 `.gocache` 验证，不要清理用户全局缓存。
