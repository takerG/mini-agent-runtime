---
name: mini-agent-runtime
description: Use when working in the mini-agent-runtime Go repository, especially changes to agent runtime architecture, CLI modes, streaming model calls, ToolRegistry tools, planner/executor flow, memory, trace, errors, prompts, tests, documentation, or CRLF/project constraints.
---

# Mini Agent Runtime

先读取仓库根目录的 `AGENTS.md`，再按任务范围读取本 skill 的 references。不要依赖历史对话记忆来恢复项目规则。

## 工作流程

1. 明确本次变更影响的边界：CLI、agent runtime、model client、tools、planner、executor、memory、trace、errors、server 或 docs。
2. 读取 `references/project-constraints.md`，确认必须遵守的长期约束。
3. 涉及架构边界或包职责时，读取 `references/architecture.md`。
4. 涉及测试、构建、函数注释或换行时，读取 `references/verification.md`。
5. 实现时保持变更聚焦，不做无关重构；遇到用户已有改动时保留并与之协作。

## 实现原则

- 优先沿用现有包结构、命名和测试风格。
- 新能力先找合适组件边界，再接入 `main.go` 的依赖装配。
- 为学习目的保留清晰中文注释，但避免无意义重复解释。
- 工具、memory、trace、errors、planner/executor 都按可扩展组件设计，不把 demo 逻辑写死在 CLI 中。

## References

- `references/project-constraints.md`：完整项目约束。
- `references/architecture.md`：包职责和扩展边界。
- `references/verification.md`：验证命令和检查清单。
