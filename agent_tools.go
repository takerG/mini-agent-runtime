package main

import (
	"fmt"
	"strconv"
	"time"
)

// Tool 是 agent 工具系统的核心抽象。
//
// 一个工具需要同时回答两个问题：
//  1. Definition: 如何向模型描述自己，让模型知道什么时候、怎么调用它。
//  2. Execute: 当模型真的发起 tool call 时，Go 代码如何执行它。
//
// 这样新增工具时，只需要新增一个实现 Tool 的类型，然后注册实例即可。
type Tool interface {
	Definition() toolDefinition
	Execute(args map[string]any) (string, error)
}

type ToolRegistry struct {
	order []string
	tools map[string]Tool
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		order: []string{},
		tools: map[string]Tool{},
	}
}

func (r *ToolRegistry) Register(tool Tool) error {
	if r == nil {
		return fmt.Errorf("tool registry is nil")
	}
	if tool == nil {
		return fmt.Errorf("tool is nil")
	}

	definition := tool.Definition()
	name := definition.Function.Name
	if name == "" {
		return fmt.Errorf("tool name is required")
	}
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %s already registered", name)
	}

	r.order = append(r.order, name)
	r.tools[name] = tool
	return nil
}

func (r *ToolRegistry) Definitions() []toolDefinition {
	if r == nil {
		return nil
	}

	definitions := make([]toolDefinition, 0, len(r.order))
	for _, name := range r.order {
		definitions = append(definitions, r.tools[name].Definition())
	}
	return definitions
}

func (r *ToolRegistry) Execute(call toolCall) (string, error) {
	if r == nil {
		return "", fmt.Errorf("tool registry is nil")
	}

	tool, ok := r.tools[call.Function.Name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", call.Function.Name)
	}
	return tool.Execute(call.Function.Arguments)
}

func DefaultToolRegistry(now func() time.Time) *ToolRegistry {
	registry := NewToolRegistry()
	mustRegisterTool(registry, CurrentTimeAgentTool{Now: now})
	mustRegisterTool(registry, CalculatorAgentTool{})
	return registry
}

func AvailableTools() []toolDefinition {
	return DefaultToolRegistry(time.Now).Definitions()
}

func ExecuteToolCall(call toolCall, now func() time.Time) (string, error) {
	return DefaultToolRegistry(now).Execute(call)
}

func mustRegisterTool(registry *ToolRegistry, tool Tool) {
	if err := registry.Register(tool); err != nil {
		panic(err)
	}
}

type CurrentTimeAgentTool struct {
	Now func() time.Time
}

func (t CurrentTimeAgentTool) Definition() toolDefinition {
	return toolDefinition{
		Type: "function",
		Function: toolDescription{
			Name:        "current_time",
			Description: "获取当前系统时间。当用户询问现在几点、今天日期、当前时间时使用。",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
}

func (t CurrentTimeAgentTool) Execute(map[string]any) (string, error) {
	return CurrentTimeTool(t.Now), nil
}

type CalculatorAgentTool struct{}

func (CalculatorAgentTool) Definition() toolDefinition {
	return toolDefinition{
		Type: "function",
		Function: toolDescription{
			Name:        "calculator",
			Description: "执行两个数字之间的四则运算。当用户需要精确计算加减乘除时使用。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"op": map[string]any{
						"type":        "string",
						"description": "运算符，只能是 +、-、*、/",
						"enum":        []string{"+", "-", "*", "/"},
					},
					"a": map[string]any{
						"type":        "number",
						"description": "左操作数",
					},
					"b": map[string]any{
						"type":        "number",
						"description": "右操作数",
					},
				},
				"required": []string{"op", "a", "b"},
			},
		},
	}
}

func (CalculatorAgentTool) Execute(args map[string]any) (string, error) {
	op, err := stringArgument("calculator", args, "op")
	if err != nil {
		return "", err
	}
	a, err := floatArgument("calculator", args, "a")
	if err != nil {
		return "", err
	}
	b, err := floatArgument("calculator", args, "b")
	if err != nil {
		return "", err
	}
	result, err := CalculatorTool(op, a, b)
	if err != nil {
		return "", err
	}
	return strconv.FormatFloat(result, 'f', -1, 64), nil
}

func stringArgument(toolName string, args map[string]any, name string) (string, error) {
	value, ok := args[name]
	if !ok {
		return "", fmt.Errorf("tool %s missing argument %s", toolName, name)
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("tool %s argument %s must be string", toolName, name)
	}
	return text, nil
}

func floatArgument(toolName string, args map[string]any, name string) (float64, error) {
	value, ok := args[name]
	if !ok {
		return 0, fmt.Errorf("tool %s missing argument %s", toolName, name)
	}
	number, ok := value.(float64)
	if !ok {
		return 0, fmt.Errorf("tool %s argument %s must be number", toolName, name)
	}
	return number, nil
}
