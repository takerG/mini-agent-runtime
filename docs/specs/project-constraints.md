# Project Constraints Spec

本文件是面向人类维护者的长期约束镜像。Codex 自动读取的主入口是仓库根目录的 `AGENTS.md`，详细规则维护在 `.codex/skills/mini-agent-runtime/references/` 中。

## Canonical Files

- `AGENTS.md`：Codex 自动发现的根项目指令文件，负责声明必须遵守的核心规则和读取顺序。
- `.codex/skills/mini-agent-runtime/SKILL.md`：项目本地 skill 入口，描述什么时候使用 mini-agent-runtime 的工程规范。
- `.codex/skills/mini-agent-runtime/references/project-constraints.md`：完整项目约束。
- `.codex/skills/mini-agent-runtime/references/architecture.md`：包职责和扩展边界。
- `.codex/skills/mini-agent-runtime/references/verification.md`：测试、构建、换行和文档检查命令。

## Maintenance Rule

后续新增长期约束时，优先更新 `.codex/skills/mini-agent-runtime/references/project-constraints.md`。如果约束会影响所有 Codex 会话的第一步行为，再同步更新 `AGENTS.md`。
