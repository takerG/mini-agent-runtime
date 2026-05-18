package errors

import (
	stderrors "errors"
	"fmt"
	"io"
	"strings"
)

// Code 是稳定的错误分类，适合日志检索、面试展示、未来指标统计，也适合交给模型理解失败类型。
type Code string

const (
	CodeUnknown                  Code = "unknown"
	CodeToolNotFound             Code = "tool_not_found"
	CodeToolInvalidArguments     Code = "tool_invalid_arguments"
	CodeToolExecutionFailed      Code = "tool_execution_failed"
	CodeHumanApprovalRequired    Code = "human_approval_required"
	CodeHumanApprovalDenied      Code = "human_approval_denied"
	CodeHumanApprovalExpired     Code = "human_approval_expired"
	CodeModelRequestFailed       Code = "model_request_failed"
	CodeUpstreamRequestFailed    Code = "upstream_request_failed"
	CodeUpstreamStatusFailed     Code = "upstream_status_failed"
	CodeStreamDecodeFailed       Code = "stream_decode_failed"
	CodeStreamReadFailed         Code = "stream_read_failed"
	CodeStreamWriteFailed        Code = "stream_write_failed"
	CodeHTTPProxyFailed          Code = "http_proxy_failed"
	CodeInvalidUserInput         Code = "invalid_user_input"
	CodeConversationLimit        Code = "conversation_limit"
	CodeRequestBuildFailed       Code = "request_build_failed"
	CodeResponseCloseFailed      Code = "response_close_failed"
	CodeCalculatorDivisionByZero Code = "calculator_division_by_zero"
)

// Node 标识错误发生或经过的运行时节点。
// Wrap 时从外到内形成调用链，OriginNode 会返回最内层，也就是实际错误发生节点。
type Node string

const (
	NodeMain          Node = "main"
	NodeAgentLoop     Node = "agent.loop"
	NodeAgentToolCall Node = "agent.tool_call"
	NodeApproval      Node = "approval"
	NodeMemory        Node = "memory"
	NodeModelClient   Node = "model.client"
	NodeOllamaClient  Node = "ollama.client"
	NodeOllamaStream  Node = "ollama.stream"
	NodeServerProxy   Node = "server.proxy"
	NodeToolRegistry  Node = "tools.registry"
	NodeCalculator    Node = "tools.calculator"
	NodeCurrentTime   Node = "tools.current_time"
)

type AppError struct {
	Code    Code
	Node    Node
	Message string
	Cause   error
}

// New 创建不包含底层 cause 的应用错误。
func New(node Node, code Code, message string) *AppError {
	return &AppError{
		Code:    code,
		Node:    node,
		Message: message,
	}
}

// Wrap 用统一错误码和运行节点包装底层错误，保留调用链上下文。
func Wrap(node Node, code Code, cause error, message string) *AppError {
	if cause == nil {
		return New(node, code, message)
	}
	return &AppError{
		Code:    code,
		Node:    node,
		Message: message,
		Cause:   cause,
	}
}

// Error 返回面向日志和调试的错误文本。
func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

// Unwrap 返回底层 cause，使 errors.Is 和 errors.As 能继续沿错误链工作。
func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// AsAppError 尝试从任意 error 中提取 AppError。
func AsAppError(err error) *AppError {
	var appErr *AppError
	if stderrors.As(err, &appErr) {
		return appErr
	}
	return nil
}

// NodeChain 按从外到内的顺序提取错误经过的运行节点链路。
func NodeChain(err error) []string {
	var chain []string
	for err != nil {
		if appErr := AsAppError(err); appErr != nil {
			chain = append(chain, string(appErr.Node))
			err = appErr.Cause
			continue
		}
		break
	}
	return chain
}

// OriginNode 返回错误链最内层的运行节点，也就是最接近真实发生位置的节点。
func OriginNode(err error) Node {
	var origin Node
	for err != nil {
		if appErr := AsAppError(err); appErr != nil {
			origin = appErr.Node
			err = appErr.Cause
			continue
		}
		break
	}
	return origin
}

// FormatForModel 将错误整理成模型容易理解的工具 observation 文本。
func FormatForModel(err error) string {
	info := collectInfo(err)
	return fmt.Sprintf(
		"tool error: code=%s origin=%s node_chain=%s message=%s detail=%s",
		info.Code,
		info.Origin,
		info.NodeChain,
		info.Message,
		info.Detail,
	)
}

type Reporter struct {
	debug  bool
	writer io.Writer
}

// NewReporter 创建统一错误 reporter，用于普通日志和 debug 明细输出。
func NewReporter(debug bool, writer io.Writer) *Reporter {
	return &Reporter{
		debug:  debug,
		writer: writer,
	}
}

// Log 将错误按面向操作者的格式写入 reporter 输出流。
func (r *Reporter) Log(err error) {
	if r == nil || r.writer == nil || err == nil {
		return
	}
	_, _ = fmt.Fprintf(r.writer, "[error] %s\n", formatForOperator(err))
}

// Debug 在 debug 模式开启时打印更适合排障的错误明细。
func (r *Reporter) Debug(err error) {
	if r == nil || !r.debug || r.writer == nil || err == nil {
		return
	}
	_, _ = fmt.Fprintf(r.writer, "[debug] error: %s\n", formatForOperator(err))
}

// formatForOperator 将错误格式化成人类排障时更容易扫描的一行文本。
func formatForOperator(err error) string {
	info := collectInfo(err)
	return fmt.Sprintf(
		"code=%s origin=%s node_chain=%s message=%s detail=%s",
		info.Code,
		info.Origin,
		info.NodeChain,
		info.Message,
		info.Detail,
	)
}

type errorInfo struct {
	Code      Code
	Origin    Node
	NodeChain string
	Message   string
	Detail    string
}

// collectInfo 从错误链中提取统一错误码、发生节点、节点链路和细节信息。
func collectInfo(err error) errorInfo {
	info := errorInfo{
		Code:      CodeUnknown,
		Origin:    Node("unknown"),
		NodeChain: "unknown",
		Message:   fmt.Sprint(err),
		Detail:    fmt.Sprint(err),
	}
	if err == nil {
		info.Message = ""
		info.Detail = ""
		return info
	}

	if appErr := AsAppError(err); appErr != nil {
		info.Code = appErr.Code
		info.Message = appErr.Message
		if origin := OriginNode(err); origin != "" {
			info.Origin = origin
		}
		if chain := NodeChain(err); len(chain) > 0 {
			info.NodeChain = strings.Join(chain, " > ")
		}
		info.Detail = deepestMessage(err)
	}
	return info
}

// deepestMessage 返回错误链最内层的消息，帮助定位真正失败原因。
func deepestMessage(err error) string {
	detail := fmt.Sprint(err)
	for err != nil {
		if appErr := AsAppError(err); appErr != nil {
			detail = appErr.Message
			err = appErr.Cause
			continue
		}
		detail = err.Error()
		break
	}
	return detail
}
