# Coding Spec

本文件是 mini-agent-runtime 的项目级代码质量规范。`AGENTS.md` 是 Codex 自动读取入口，本文件用于沉淀更具体、可执行的 Go 质量约束。

## Go 代码规范

- 所有 Go 文件必须通过 `gofmt` 格式化。
- 所有 Go 函数和方法必须有中文 GoDoc 注释，且注释必须以函数名或方法名开头。
- 根目录只保留 `main.go` 一个 Go 入口文件；其他能力按职责放入 `internal/` 包。
- 函数职责保持单一；当参数列表过长、依赖过多或同一组参数反复出现时，优先抽象为 options/config/dependencies 结构体。
- 公共逻辑可以抽象复用，但不为了消除少量清晰重复而引入过重抽象。
- 不为 `options.Field`、`config.Field` 等简单字段访问创建一次性局部别名；只有在需要默认值改写、多次复用、命名复杂表达式、避免重复昂贵计算或显著缩短长表达式时，才引入新的局部变量。
- 核心业务编排链路不要通过匿名函数、函数字段或层层注入的 function callback 隐藏真实实现；优先使用 interface、具名结构体和具名方法表达扩展点。测试 mock、短小的标准库适配器和 `defer` 收尾函数可以例外。
- 不提交未使用变量、未使用 import、不可达代码、无效赋值、影子变量造成的可读性风险。

## Error 处理规范

- 可能被 wrap 的错误禁止直接用 `err == targetErr` 或 `err != targetErr` 判断。
- 判断错误链必须使用标准库 `errors.Is` 或 `errors.As`。
- 包装错误必须保留原因链，优先使用 `fmt.Errorf("...: %w", err)` 或项目 `internal/errors` 包提供的包装能力。
- agent 内部错误应尽量带上稳定错误码、发生节点和调用链，方便模型理解、日志排查和 `--debug` 输出。
- 工具不存在、工具执行失败、模型请求失败等业务错误应转为可理解 observation 或结构化错误，不要在可恢复对话流程中直接退出。

## Context 与资源规范

- 跨包调用、模型请求、工具执行、planner/executor 流程必须传递 `context.Context`。
- 新增 goroutine 必须有明确退出条件；不能只依赖调用方放弃等待来“模拟超时”。
- 工具超时治理应通过带 deadline 的 context 推进；工具实现必须尽快响应 `ctx.Done()`。
- HTTP response body、文件、trace sink 等资源必须关闭；如关闭失败会影响结果，应记录或返回错误。
- 流式输出必须收到 chunk 后尽快写出并 flush，不能为了日志、聚合或总结而阻塞正常 token 输出。

## Lint 与测试要求

- 每次代码变更至少运行 `go test -count=1 ./...`。
- 提交前运行统一质量门禁：`powershell -NoProfile -ExecutionPolicy Bypass -File scripts/check.ps1`。
- 如果本机有 `make`，也可以运行 `make check`。
- `scripts/check.ps1` 包含 `gofmt` 检查、CRLF 检查、`go vet ./...`、`go test ./...`、`staticcheck ./...` 和 `golangci-lint run`。
- 如果 `staticcheck` 不存在，安装：`go install honnef.co/go/tools/cmd/staticcheck@latest`。
- 如果 `golangci-lint` 不存在，按官方文档安装：https://golangci-lint.run/welcome/install/
- 文档、换行、工程配置变更后运行 `git diff --check`。

## 提交前自检流程

1. 确认本次改动聚焦，不混入无关重构或本地生成物。
2. 运行 `gofmt` 或至少通过 `gofmt` 检查。
3. 运行 `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/check.ps1`。
4. 如果缺少 `staticcheck` 或 `golangci-lint`，先安装后重跑；如果当前环境无法安装，在最终说明中明确尚未完成的检查。
5. 运行 `git diff --check`，确认没有尾随空格或换行问题。
6. 检查新增/修改函数是否保留中文 GoDoc 注释。

