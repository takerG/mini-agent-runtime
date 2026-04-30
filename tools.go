package main

import (
	"fmt"
	"time"
)

// CurrentTimeTool 是一个“获取当前时间”的孤立工具函数。
//
// 这里没有直接在函数内部调用 time.Now()，而是把 now 作为参数传进来。
// 这样做有两个好处：
//  1. 真实运行时可以传 time.Now，拿到真正的当前时间。
//  2. 测试时可以传一个固定时间，测试结果就不会随着时间变化而不稳定。
//
// 以后把它接入 agent 时，模型可以把这个工具理解为：
// “当你需要知道当前时间时，调用 CurrentTimeTool”。
func CurrentTimeTool(now func() time.Time) string {
	if now == nil {
		now = time.Now
	}
	return now().Format("2006-01-02 15:04:05 MST")
}

// CalculatorTool 是一个最小计算器工具。
//
// 第一版只支持二元四则运算：
//   - op 为 "+" 时计算 a+b
//   - op 为 "-" 时计算 a-b
//   - op 为 "*" 时计算 a*b
//   - op 为 "/" 时计算 a/b
//
// 这里返回 (float64, error)，是因为工具调用经常会失败：
// 例如未知运算符、除零、参数格式不对等。先把错误通道设计出来，
// 后面接入 agent 时就可以把错误信息返回给模型，让模型决定如何修正调用。
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
			return 0, fmt.Errorf("division by zero")
		}
		return a / b, nil
	default:
		return 0, fmt.Errorf("unknown calculator operation: %q", op)
	}
}
