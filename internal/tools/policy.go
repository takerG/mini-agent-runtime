package tools

import (
	"context"
	"errors"
	"fmt"
	"time"

	apperrors "mini-agent-runtime/internal/errors"
	"mini-agent-runtime/internal/ollama"
)

// AllowPolicy 定义工具执行前的准入检查策略。
type AllowPolicy interface {
	// Allow 判断本次工具调用是否允许执行。
	Allow(call ollama.ToolCall) error
}

// RetryPolicy 定义工具执行失败后的重试判断策略。
type RetryPolicy interface {
	// Retryable 判断本次工具错误是否允许再次尝试。
	Retryable(err error) bool
}

// ExecutionPolicy 描述工具调用的运行时治理策略。
type ExecutionPolicy struct {
	Timeout     time.Duration
	MaxAttempts int
	Allow       AllowPolicy
	Retryable   RetryPolicy
}

// DefaultExecutionPolicy 返回默认工具执行策略：单次执行、不超时、不自动重试。
func DefaultExecutionPolicy() ExecutionPolicy {
	return ExecutionPolicy{MaxAttempts: 1}
}

// ExecuteWithPolicy 在注册表中查找工具，并按照传入策略执行工具调用。
func (r *ToolRegistry) ExecuteWithPolicy(ctx context.Context, call ollama.ToolCall, policy ExecutionPolicy) (string, error) {
	tool, ok := r.tools[call.Function.Name]
	if !ok {
		return "", apperrors.New(apperrors.NodeToolRegistry, apperrors.CodeToolNotFound, fmt.Sprintf("unknown tool: %s", call.Function.Name))
	}
	if policy.Allow != nil {
		if err := policy.Allow.Allow(call); err != nil {
			return "", apperrors.Wrap(apperrors.NodeToolRegistry, apperrors.CodeToolExecutionFailed, err, fmt.Sprintf("tool execution denied: %s", call.Function.Name))
		}
	}
	policy = normalizeExecutionPolicy(policy)

	var lastErr error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		result, err := executeToolAttempt(ctx, tool, call.Function.Arguments, policy.Timeout)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if attempt >= policy.MaxAttempts || policy.Retryable == nil || !policy.Retryable.Retryable(err) {
			break
		}
	}
	return "", lastErr
}

// normalizeExecutionPolicy 补齐工具执行策略中的默认值。
func normalizeExecutionPolicy(policy ExecutionPolicy) ExecutionPolicy {
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 1
	}
	return policy
}

// executeToolAttempt 执行单次工具调用，并在配置超时时为本次调用附加 deadline。
func executeToolAttempt(ctx context.Context, tool Tool, args map[string]any, timeout time.Duration) (string, error) {
	attemptCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		attemptCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	result, err := tool.Execute(attemptCtx, args)
	if timeout > 0 && errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
		return "", apperrors.New(apperrors.NodeToolRegistry, apperrors.CodeToolExecutionFailed, fmt.Sprintf("tool %s timed out after %s", tool.Name(), timeout))
	}
	return result, err
}
