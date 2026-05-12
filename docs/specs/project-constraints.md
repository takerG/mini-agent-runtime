# Project Constraints Spec

本文档记录本项目的长期工程约束。后续任何实现、重构、测试、文档更新，都应先读取并遵守这里的规则，避免因为对话上下文压缩导致约束遗忘。

## 1. 代码与注释

- MUST 使用 Go 语言实现项目主体。
- MUST 保持入口层轻量：根目录入口只保留 `main.go` 一个 Go 入口文件，其他能力都应组件化放入 `internal/` 下的包中。
- MUST 为所有 Go 函数和方法添加符合 Go 注释规范的中文注释。
- MUST 保持函数注释以函数名或方法名开头，例如：

  ```go
  // isPlannerMode 判断是否是 Planner / Strict Planner 模式，这两种模式的交互流程和普通 Chat 模式不同。
  func isPlannerMode(mode Mode) bool {
  	return mode == ModePlan || mode == ModeStrictPlan
  }
  ```

- MUST 后续新增函数时同步增加同风格注释，不要等到集中补文档。
- SHOULD 在实现代码中保留尽可能清晰、详尽、适合学习的中文说明，但不要写无意义的重复注释。
- SHOULD 优先保持代码风格简洁、可读、可维护，避免为了抽象而抽象。
- SHOULD 避免单个函数承担过多职责；当参数列表过长或职责混杂时，应考虑抽象为 options/config 结构体或拆分组件。

## 2. 文件格式

- MUST 默认使用 CRLF 换行。
- MUST 在提交前避免出现裸 LF 换行，尤其是 `.go`、`.md`、`.mod`、`.gitignore`、`.gitattributes`、`.editorconfig`。
- MUST 不提交 `.idea/` 和构建缓存目录。
- SHOULD 使用 `gofmt` 格式化所有 Go 文件。

## 3. CLI 体验

- MUST 使用双横线 CLI 参数形式，例如 `--mode`、`--trace`、`--model`、`--think`，不要新增只有单横线语义的参数说明。
- MUST 支持多轮 CLI 对话，而不是只支持启动参数中的单轮问题。
- MUST 支持启动时通过参数传入首轮用户消息，首轮结束后继续进入交互式 CLI。
- MUST 支持 `/exit`、`/quit`、`exit`、`quit` 退出命令。
- MUST 通过启动参数传入 think 控制，不允许把 think 写死在代码里。
- MUST 保持当前已验证的 think 语义：`--think=true` 时不显示 thinking 过程，`--think=false` 时显示 thinking 过程。

## 4. 流式输出

- MUST 对本地模型使用真正的流式调用。
- MUST 向 `http://localhost:11434/api/chat` 或配置的兼容 endpoint 发送 `stream: true` 请求。
- MUST 在接收到上游 chunk 后尽快解析并输出 `response.message.content`，避免等待完整响应结束。
- MUST 在 HTTP proxy 模式下每个 chunk 后 flush，保证调用方能尽快看到内容。
- SHOULD 避免使用会限制长行的实现方式导致流式 JSON 被截断；读取流时要考虑长 thinking、长 content、长 tool arguments。

## 5. Agent 模式

- MUST 保留默认 `chat` 模式，使用模型原生 tool calling 完成普通对话。
- MUST 保留 `plan` 模式作为 Hybrid Planner/Executor：Planner 先给计划，Executor 再让模型根据计划继续使用原生 tool calling。
- MUST 保留 `strict-plan` 模式作为 Strict Planner/Executor：Planner 输出可执行 JSON，Go 解析 `tool_name` 和 `arguments`，Go 直接执行工具，模型只负责最终总结。
- MUST 在 plan 类模式下向 CLI 展示完整过程，包括 `[plan]`、`[observation]` 和 `Agent:`。
- MUST 在 strict-plan 模式下避免让模型二次决定工具调用；工具执行权归 Go runtime。
- SHOULD 在架构和命名上清晰区分 planner、executor、runtime、model client、tools、trace、errors 等职责边界。

## 6. Tools 系统

- MUST 使用 `Tool` interface 和 `ToolRegistry` 注册机制管理工具。
- MUST 通过 `registry.Definitions()` 获取模型可见工具定义。
- MUST 通过 `registry.Execute(ctx, call)` 执行工具调用。
- MUST 禁止在 CLI 或 runtime 中回退到硬编码 `switch tool_name` 的工具分发方式。
- MUST 每个 tool 使用独立 `.go` 文件维护，并使用工具名命名，例如 `calculator.go`、`current_time.go`。
- MUST 默认注册内置工具：
  - `calculator`
  - `current_time`
- MUST 当工具不存在或工具返回错误时，不直接退出 agent 对话，而是把结构化错误信息作为 observation 交给模型继续处理。
- SHOULD 后续新增工具时只新增工具实现并注册到默认注册表，避免修改核心执行流程。

## 7. Trace Log

- MUST 使用 hook/sink 形式的 trace 机制，不要只依赖在业务逻辑里零散手写日志。
- MUST 在 trace 模式下完整展示每轮请求中给模型的入参。
- MUST 在 trace 模式下完整展示模型返回的内容和工具调用。
- MUST trace 关键 agent 节点，包括用户输入、模型请求、模型响应、工具调用、工具结果、工具错误、planner、executor、最终回答。
- SHOULD trace 输出写入 stderr，避免污染 stdout 中的模型流式回答。
- SHOULD 设计 trace 事件为结构化数据，便于未来输出到文件、指标系统或 UI。
- SHOULD 记录隐私风险：trace 可能包含用户输入、工具参数和工具结果；后续扩展外部工具时应考虑脱敏和截断策略。

## 8. 错误处理

- MUST 使用统一 errors 包接管错误处理能力。
- MUST 维护稳定的错误码、运行节点和调用链信息。
- MUST 提供面向模型理解的错误格式化能力，便于工具失败后交还给模型处理。
- MUST 提供面向操作者的日志输出能力。
- MUST 通过 `--debug` 控制 debug 信息是否打印到命令行。
- MUST debug/error 日志标注错误真实发生节点和传播链路。

## 9. 架构边界

- MUST 保持 `main.go` 只负责参数解析、模式选择和依赖装配。
- MUST 保持 `internal/agent` 负责 CLI loop、runtime 编排、历史消息和模式分发。
- MUST 保持 `internal/model` 负责共享模型调用、HTTP 请求、trace 和响应捕获。
- MUST 保持 `internal/ollama` 负责 Ollama 兼容协议类型、请求构造和流式解析。
- MUST 保持 `internal/tools` 负责工具接口、注册表、内置工具和工具参数校验。
- MUST 保持 `internal/planner` 负责 planner 输出结构和解析。
- MUST 保持 `internal/executor` 负责 Hybrid Planner/Executor 的执行阶段。
- MUST 保持 `internal/trace` 负责 trace 事件、hooks 和 logger sink。
- MUST 保持 `internal/errors` 负责错误码、节点、格式化、日志和 debug 输出。
- MUST 保持 `internal/server` 负责 HTTP streaming proxy。

## 10. 验证要求

- MUST 在代码变更后优先运行：

  ```powershell
  $env:GOCACHE = "D:\work\mini-agent-runtime\.gocache"
  go test -count=1 ./...
  go build -buildvcs=false ./...
  ```

- MUST 在文档或换行相关变更后至少检查：

  ```powershell
  git diff --check
  ```

- SHOULD 在涉及函数新增、重命名或大规模改动后检查函数注释覆盖。
- SHOULD 若默认 Go 全局 build cache 异常，优先使用项目内 `.gocache` 验证，不要直接清理用户全局缓存。

