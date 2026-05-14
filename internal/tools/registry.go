package tools

import (
	"context"
	"time"

	"mini-agent-runtime/internal/ollama"
)

// Tool 是 agent 工具系统的核心抽象。
//
// 一个工具同时面向两个使用者：
//   - 模型通过 Definition 理解工具能力、使用时机和参数结构。
//   - Go 运行时通过 Execute 执行真实逻辑，并把结果交还给模型。
//
// Name 和 Description 单独暴露，方便注册表、日志、测试和未来 UI 面板读取工具元信息。
// Execute 接收 context.Context，为超时、取消、trace id、用户会话 id 等工程能力预留入口。
type Tool interface {
	Name() string
	Description() string
	Definition() ollama.ToolDefinition
	Execute(ctx context.Context, args map[string]any) (string, error)
}

// ToolRegistry 保存“工具名 -> 工具实现”的映射。
//
// 新增工具时，只需要新增一个以工具名命名的 .go 文件，实现 Tool 接口，
// 再在 NewDefaultToolRegistry 里注册实例即可。
type ToolRegistry struct {
	tools map[string]Tool
}

// NewToolRegistry 创建空工具注册表，调用方可以按需注册 Tool 实现。
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

// Register 将工具实现注册到工具表中，同名工具会被后注册的实现覆盖。
func (r *ToolRegistry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

// Definitions 返回所有已注册工具的模型可见定义。
func (r *ToolRegistry) Definitions() []ollama.ToolDefinition {
	definitions := make([]ollama.ToolDefinition, 0, len(r.tools))
	for _, tool := range r.tools {
		definitions = append(definitions, tool.Definition())
	}
	return definitions
}

// Execute 根据模型工具调用名称查找工具并执行。
func (r *ToolRegistry) Execute(ctx context.Context, call ollama.ToolCall) (string, error) {
	return r.ExecuteWithPolicy(ctx, call, DefaultExecutionPolicy())
}

// NewDefaultToolRegistry 注册当前版本内置的默认工具集合。
func NewDefaultToolRegistry(now func() time.Time) *ToolRegistry {
	registry := NewToolRegistry()
	registry.Register(NewCurrentTimeTool(now))
	registry.Register(NewCalculatorTool())
	return registry
}
