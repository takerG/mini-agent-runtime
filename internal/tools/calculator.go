package tools

import (
	"context"
	"fmt"
	"strconv"

	apperrors "mini-agent-runtime/internal/errors"
	"mini-agent-runtime/internal/ollama"
)

type CalculatorAgentTool struct{}

func NewCalculatorTool() Tool {
	return CalculatorAgentTool{}
}

func (CalculatorAgentTool) Name() string {
	return "calculator"
}

func (CalculatorAgentTool) Description() string {
	return "执行两个数字之间的四则运算。当用户需要精确计算加减乘除时使用。"
}

func (t CalculatorAgentTool) Definition() ollama.ToolDefinition {
	return ollama.ToolDefinition{
		Type: "function",
		Function: ollama.ToolDescription{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"op": map[string]any{
						"type":        "string",
						"description": "运算符，只能是 +、-、*、/。",
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

func (CalculatorAgentTool) Execute(_ context.Context, args map[string]any) (string, error) {
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
		return "", apperrors.Wrap(apperrors.NodeCalculator, apperrors.CodeToolInvalidArguments, err, "calculator failed")
	}
	return strconv.FormatFloat(result, 'f', -1, 64), nil
}

// CalculatorTool 是一个最小计算器工具函数，只负责二元四则运算。
func CalculatorTool(op string, a float64, b float64) (float64, error) {
	switch op {
	case "+":
		return a + b, nil
	case "-":
		return a - b, nil
	case "*":
		return a * b, nil
	case "/":
		if b == 0 {
			return 0, apperrors.New(apperrors.NodeCalculator, apperrors.CodeCalculatorDivisionByZero, "division by zero")
		}
		return a / b, nil
	default:
		return 0, apperrors.New(apperrors.NodeCalculator, apperrors.CodeToolInvalidArguments, fmt.Sprintf("unknown calculator operation: %q", op))
	}
}

func stringArgument(toolName string, args map[string]any, name string) (string, error) {
	value, ok := args[name]
	if !ok {
		return "", apperrors.New(apperrors.NodeCalculator, apperrors.CodeToolInvalidArguments, fmt.Sprintf("tool %s missing argument %s", toolName, name))
	}
	text, ok := value.(string)
	if !ok {
		return "", apperrors.New(apperrors.NodeCalculator, apperrors.CodeToolInvalidArguments, fmt.Sprintf("tool %s argument %s must be string", toolName, name))
	}
	return text, nil
}

func floatArgument(toolName string, args map[string]any, name string) (float64, error) {
	value, ok := args[name]
	if !ok {
		return 0, apperrors.New(apperrors.NodeCalculator, apperrors.CodeToolInvalidArguments, fmt.Sprintf("tool %s missing argument %s", toolName, name))
	}
	number, ok := value.(float64)
	if !ok {
		return 0, apperrors.New(apperrors.NodeCalculator, apperrors.CodeToolInvalidArguments, fmt.Sprintf("tool %s argument %s must be number", toolName, name))
	}
	return number, nil
}
