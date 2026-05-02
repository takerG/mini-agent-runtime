package main

import (
	"fmt"
	"strconv"
	"time"
)

func AvailableTools() []toolDefinition {
	return []toolDefinition{
		{
			Type: "function",
			Function: toolDescription{
				Name:        "current_time",
				Description: "获取当前系统时间。当用户询问现在几点、今天日期、当前时间时使用。",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		},
		{
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
		},
	}
}

func ExecuteToolCall(call toolCall, now func() time.Time) (string, error) {
	switch call.Function.Name {
	case "current_time":
		return CurrentTimeTool(now), nil
	case "calculator":
		op, err := toolStringArgument(call, "op")
		if err != nil {
			return "", err
		}
		a, err := toolFloatArgument(call, "a")
		if err != nil {
			return "", err
		}
		b, err := toolFloatArgument(call, "b")
		if err != nil {
			return "", err
		}
		result, err := CalculatorTool(op, a, b)
		if err != nil {
			return "", err
		}
		return strconv.FormatFloat(result, 'f', -1, 64), nil
	default:
		return "", fmt.Errorf("unknown tool: %s", call.Function.Name)
	}
}

func toolStringArgument(call toolCall, name string) (string, error) {
	value, ok := call.Function.Arguments[name]
	if !ok {
		return "", fmt.Errorf("tool %s missing argument %s", call.Function.Name, name)
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("tool %s argument %s must be string", call.Function.Name, name)
	}
	return text, nil
}

func toolFloatArgument(call toolCall, name string) (float64, error) {
	value, ok := call.Function.Arguments[name]
	if !ok {
		return 0, fmt.Errorf("tool %s missing argument %s", call.Function.Name, name)
	}
	number, ok := value.(float64)
	if !ok {
		return 0, fmt.Errorf("tool %s argument %s must be number", call.Function.Name, name)
	}
	return number, nil
}
