package executor

import (
	"io"

	apperrors "mini-agent-runtime/internal/errors"
	"mini-agent-runtime/internal/lifecycle"
	"mini-agent-runtime/internal/tools"
	tracing "mini-agent-runtime/internal/trace"
)

// Dependencies 保存 executor 从 runtime 层接收的共享依赖。
type Dependencies struct {
	Registry     *tools.ToolRegistry
	ToolPolicy   tools.ExecutionPolicy
	Trace        *tracing.TraceHooks
	Reporter     *apperrors.Reporter
	Stdout       io.Writer
	Recorder     *lifecycle.Recorder
	ParentStepID string
}

// normalizeDependencies 只处理 executor 本地安全默认值，不创建 runtime 层共享依赖。
func normalizeDependencies(dependencies Dependencies) Dependencies {
	if dependencies.Stdout == nil {
		dependencies.Stdout = io.Discard
	}
	return dependencies
}

// missingRegistryError 返回 executor 缺少工具注册表时的配置错误。
func missingRegistryError(component string) error {
	return apperrors.New(apperrors.NodeAgentLoop, apperrors.CodeToolExecutionFailed, component+" requires tool registry")
}
