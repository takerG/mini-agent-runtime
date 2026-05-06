package trace

import (
	"fmt"
	"io"
)

type TraceEventName string

const (
	TraceChatLoopStart TraceEventName = "chat_loop_start"
	TraceChatLoopExit  TraceEventName = "chat_loop_exit"
	TraceTurnInput     TraceEventName = "turn_input"
	TraceModelRequest  TraceEventName = "model_request"
	TraceModelResponse TraceEventName = "model_response"
	TraceToolCall      TraceEventName = "tool_call"
	TraceToolResult    TraceEventName = "tool_result"
	TraceToolError     TraceEventName = "tool_error"
	TraceFinalAnswer   TraceEventName = "final_answer"
)

type TraceEvent struct {
	Name TraceEventName
	Data any
}

type TraceSink interface {
	Emit(event TraceEvent)
}

type TraceHooks struct {
	sink TraceSink
}

func NewTraceHooks(sink TraceSink) *TraceHooks {
	return &TraceHooks{sink: sink}
}

func (h *TraceHooks) emit(name TraceEventName, data any) {
	if h == nil || h.sink == nil {
		return
	}
	h.sink.Emit(TraceEvent{Name: name, Data: data})
}

type ChatLoopStartTrace struct {
	Endpoint string
	Model    string
	Think    bool
	Tools    int
}

type ChatLoopExitTrace struct {
	Command string
}

type TurnInputTrace struct {
	Message         string
	HistoryMessages int
}

type ModelRequestTrace struct {
	ToolRound int
	Messages  int
	Tools     int
}

type ModelResponseTrace struct {
	ToolRound    int
	ContentChars int
	ToolCalls    int
}

type ToolCallTrace struct {
	Name      string
	Arguments map[string]any
}

type ToolResultTrace struct {
	Name   string
	Result string
}

type ToolErrorTrace struct {
	Name  string
	Error error
}

type FinalAnswerTrace struct {
	ContentChars    int
	HistoryMessages int
}

func (h *TraceHooks) ChatLoopStart(data ChatLoopStartTrace) {
	h.emit(TraceChatLoopStart, data)
}

func (h *TraceHooks) ChatLoopExit(data ChatLoopExitTrace) {
	h.emit(TraceChatLoopExit, data)
}

func (h *TraceHooks) TurnInput(data TurnInputTrace) {
	h.emit(TraceTurnInput, data)
}

func (h *TraceHooks) ModelRequest(data ModelRequestTrace) {
	h.emit(TraceModelRequest, data)
}

func (h *TraceHooks) ModelResponse(data ModelResponseTrace) {
	h.emit(TraceModelResponse, data)
}

func (h *TraceHooks) ToolCall(data ToolCallTrace) {
	h.emit(TraceToolCall, data)
}

func (h *TraceHooks) ToolResult(data ToolResultTrace) {
	h.emit(TraceToolResult, data)
}

func (h *TraceHooks) ToolError(data ToolErrorTrace) {
	h.emit(TraceToolError, data)
}

func (h *TraceHooks) FinalAnswer(data FinalAnswerTrace) {
	h.emit(TraceFinalAnswer, data)
}

type TraceLogger struct {
	enabled bool
	writer  io.Writer
}

func NewTraceLogger(enabled bool, writer io.Writer) *TraceLogger {
	return &TraceLogger{
		enabled: enabled,
		writer:  writer,
	}
}

func (t *TraceLogger) Emit(event TraceEvent) {
	if t == nil || !t.enabled || t.writer == nil {
		return
	}
	fmt.Fprintf(t.writer, "[trace] %s: %s\n", event.Name, formatTraceData(event.Data))
}

func (t *TraceLogger) Log(step string, format string, args ...any) {
	if t == nil || !t.enabled || t.writer == nil {
		return
	}
	message := fmt.Sprintf(format, args...)
	fmt.Fprintf(t.writer, "[trace] %s: %s\n", step, message)
}

func formatTraceData(data any) string {
	switch value := data.(type) {
	case ChatLoopStartTrace:
		return fmt.Sprintf("endpoint=%s model=%s think=%v tools=%d", value.Endpoint, value.Model, value.Think, value.Tools)
	case ChatLoopExitTrace:
		return fmt.Sprintf("command=%q", value.Command)
	case TurnInputTrace:
		return fmt.Sprintf("message=%q history_messages=%d", value.Message, value.HistoryMessages)
	case ModelRequestTrace:
		return fmt.Sprintf("tool_round=%d messages=%d tools=%d", value.ToolRound, value.Messages, value.Tools)
	case ModelResponseTrace:
		return fmt.Sprintf("tool_round=%d content_chars=%d tool_calls=%d", value.ToolRound, value.ContentChars, value.ToolCalls)
	case ToolCallTrace:
		return fmt.Sprintf("name=%s arguments=%v", value.Name, value.Arguments)
	case ToolResultTrace:
		return fmt.Sprintf("name=%s result=%s", value.Name, value.Result)
	case ToolErrorTrace:
		return fmt.Sprintf("name=%s error=%v", value.Name, value.Error)
	case FinalAnswerTrace:
		return fmt.Sprintf("content_chars=%d history_messages=%d", value.ContentChars, value.HistoryMessages)
	default:
		return fmt.Sprintf("%v", data)
	}
}
