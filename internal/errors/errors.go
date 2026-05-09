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

func New(node Node, code Code, message string) *AppError {
	return &AppError{
		Code:    code,
		Node:    node,
		Message: message,
	}
}

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

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func AsAppError(err error) *AppError {
	var appErr *AppError
	if stderrors.As(err, &appErr) {
		return appErr
	}
	return nil
}

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

func NewReporter(debug bool, writer io.Writer) *Reporter {
	return &Reporter{
		debug:  debug,
		writer: writer,
	}
}

func (r *Reporter) Log(err error) {
	if r == nil || r.writer == nil || err == nil {
		return
	}
	fmt.Fprintf(r.writer, "[error] %s\n", formatForOperator(err))
}

func (r *Reporter) Debug(err error) {
	if r == nil || !r.debug || r.writer == nil || err == nil {
		return
	}
	fmt.Fprintf(r.writer, "[debug] error: %s\n", formatForOperator(err))
}

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
