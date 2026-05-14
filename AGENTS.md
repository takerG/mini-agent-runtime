# AGENTS.md

本文件是 mini-agent-runtime 仓库的 Codex 主入口约束。Codex 官方约定使用 `AGENTS.md`（复数）作为可自动发现的项目指令文件；不要改名为 `AGENT.md`。

## 必读顺序

1. 修改任何代码、测试、文档或工程结构前，先读取本文件。
2. 需要了解完整项目约束时，读取 `.codex/skills/mini-agent-runtime/SKILL.md`。
3. 需要改动架构、CLI、tools、memory、trace、errors、prompts 或换行策略时，继续读取 `.codex/skills/mini-agent-runtime/references/project-constraints.md`。
4. 需要验证变更时，读取 `.codex/skills/mini-agent-runtime/references/verification.md`。

## 核心约束

- 入口层保持轻量：根目录只保留 `main.go` 一个 Go 入口文件，其他能力必须组件化放入 `internal/` 下的包。
- 默认使用 CRLF 换行；不要提交 `.idea/`、`.gocache/`、可执行文件或其他本地生成物。
- 所有 Go 函数和方法必须有中文 GoDoc 注释，且注释必须以函数名或方法名开头。
- CLI 参数使用双横线形式，例如 `--mode`、`--trace`、`--trace-jsonl`、`--debug`、`--model`、`--think`。
- 保持 `chat`、`plan`、`strict-plan` 三种模式的职责边界；不要把 planner、executor、runtime、model client、tools、lifecycle、trace、errors 混在一起。
- 每次用户请求都必须能落到统一的 run lifecycle：`Run`、`Step`、`Observation`、`Result`。
- tools 必须通过 `Tool` interface 和 `ToolRegistry` 注册、定义、执行；禁止在 runtime 或 CLI 中回退到 `switch tool_name` 分发。
- memory 必须区分 user memory 和 session memory，并通过代码组合或切换实现，不通过 CLI 参数暴露策略细节。
- trace 必须通过 hook/sink 挂靠关键节点，并在 `--trace` 下展示每轮模型入参、模型返回、工具调用和 observation；JSONL trace 事件必须带上可关联的 run/step 上下文。
- errors 包统一负责错误码、节点、格式化、日志和 `--debug` 输出。
- 流式输出必须保持真正 streaming：收到模型 chunk 后尽快解析、输出和 flush，不要等待完整响应结束。

## 官方文件布局

- `AGENTS.md`：Codex 自动读取的根项目指令文件，保持短而关键。
- `.codex/skills/mini-agent-runtime/SKILL.md`：项目本地 skill 入口，描述何时使用本项目约束。
- `.codex/skills/mini-agent-runtime/references/`：skill 的详细参考资料，按主题拆分。
- `docs/specs/project-constraints.md`：面向人类维护者的长期约束镜像，内容指向同一套规范。

## 常用验证

在代码变更后优先运行：

```powershell
$env:GOCACHE = Join-Path (Get-Location) ".gocache"
go test -count=1 ./...
go build -buildvcs=false ./...
```

在文档、换行或工程文件变更后至少运行：

```powershell
git diff --check
```
