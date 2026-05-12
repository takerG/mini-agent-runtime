package trace

import (
	"encoding/json"
	"fmt"
	"io"

	"mini-agent-runtime/internal/ollama"
)

type TraceEventName string

const (
	TraceChatLoopStart   TraceEventName = "chat_loop_start"
	TraceChatLoopExit    TraceEventName = "chat_loop_exit"
	TraceTurnInput       TraceEventName = "turn_input"
	TraceModelRequest    TraceEventName = "model_request"
	TraceModelResponse   TraceEventName = "model_response"
	TraceToolCall        TraceEventName = "tool_call"
	TraceToolResult      TraceEventName = "tool_result"
	TraceToolError       TraceEventName = "tool_error"
	TracePlannerRequest  TraceEventName = "planner_request"
	TracePlannerResponse TraceEventName = "planner_response"
	TraceExecutorStart   TraceEventName = "executor_start"
	TraceExecutorStep    TraceEventName = "executor_step"
	TraceExecutorFinish  TraceEventName = "executor_finish"
	TraceFinalAnswer     TraceEventName = "final_answer"
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

// NewTraceHooks 创建 trace hook 集合，用于把关键运行节点事件统一发送到 sink。
func NewTraceHooks(sink TraceSink) *TraceHooks {
	return &TraceHooks{sink: sink}
}

// emit 在 hook 可用时发送 trace 事件，空 hook 会被安全忽略。
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
	Phase     string
	Request   ollama.ChatRequest
}

type ModelResponseTrace struct {
	ToolRound int
	Phase     string
	Content   string
	ToolCalls []ollama.ToolCall
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

type PlannerRequestTrace struct {
	Message      string
	MessageChars int
}

type PlannerResponseTrace struct {
	Goal  string
	Steps int
}

type ExecutorStartTrace struct {
	Steps int
}

type ExecutorStepTrace struct {
	Index    int
	Task     string
	ToolHint string
}

type ExecutorFinishTrace struct {
	ContentChars int
}

type FinalAnswerTrace struct {
	ContentChars    int
	HistoryMessages int
}

// ChatLoopStart 记录 CLI 对话循环启动事件。
func (h *TraceHooks) ChatLoopStart(data ChatLoopStartTrace) {
	h.emit(TraceChatLoopStart, data)
}

// ChatLoopExit 记录 CLI 对话循环退出事件。
func (h *TraceHooks) ChatLoopExit(data ChatLoopExitTrace) {
	h.emit(TraceChatLoopExit, data)
}

// TurnInput 记录单轮用户输入事件。
func (h *TraceHooks) TurnInput(data TurnInputTrace) {
	h.emit(TraceTurnInput, data)
}

// ModelRequest 记录发送给模型的完整请求事件。
func (h *TraceHooks) ModelRequest(data ModelRequestTrace) {
	h.emit(TraceModelRequest, data)
}

// ModelResponse 记录模型返回的文本和工具调用事件。
func (h *TraceHooks) ModelResponse(data ModelResponseTrace) {
	h.emit(TraceModelResponse, data)
}

// ToolCall 记录即将执行的工具调用事件。
func (h *TraceHooks) ToolCall(data ToolCallTrace) {
	h.emit(TraceToolCall, data)
}

// ToolResult 记录工具调用成功后的返回结果事件。
func (h *TraceHooks) ToolResult(data ToolResultTrace) {
	h.emit(TraceToolResult, data)
}

// ToolError 记录工具调用失败事件。
func (h *TraceHooks) ToolError(data ToolErrorTrace) {
	h.emit(TraceToolError, data)
}

// PlannerRequest 记录 planner 阶段收到的用户目标事件。
func (h *TraceHooks) PlannerRequest(data PlannerRequestTrace) {
	h.emit(TracePlannerRequest, data)
}

// PlannerResponse 记录 planner 阶段生成计划后的摘要事件。
func (h *TraceHooks) PlannerResponse(data PlannerResponseTrace) {
	h.emit(TracePlannerResponse, data)
}

// ExecutorStart 记录 executor 阶段启动事件。
func (h *TraceHooks) ExecutorStart(data ExecutorStartTrace) {
	h.emit(TraceExecutorStart, data)
}

// ExecutorStep 记录 executor 将要处理的计划步骤事件。
func (h *TraceHooks) ExecutorStep(data ExecutorStepTrace) {
	h.emit(TraceExecutorStep, data)
}

// ExecutorFinish 记录 executor 阶段完成事件。
func (h *TraceHooks) ExecutorFinish(data ExecutorFinishTrace) {
	h.emit(TraceExecutorFinish, data)
}

// FinalAnswer 记录一轮对话最终回答进入历史后的事件。
func (h *TraceHooks) FinalAnswer(data FinalAnswerTrace) {
	h.emit(TraceFinalAnswer, data)
}

type TraceLogger struct {
	enabled bool
	writer  io.Writer
}

// NewTraceLogger 创建把 trace 事件写入文本流的 logger。
func NewTraceLogger(enabled bool, writer io.Writer) *TraceLogger {
	return &TraceLogger{
		enabled: enabled,
		writer:  writer,
	}
}

// Emit 按统一格式输出单个 trace 事件。
func (t *TraceLogger) Emit(event TraceEvent) {
	if t == nil || !t.enabled || t.writer == nil {
		return
	}
	fmt.Fprintf(t.writer, "[trace] %s: %s\n", event.Name, formatTraceData(event.Data))
}

// Log 保留兼容旧调用的格式化 trace 日志输出能力。
func (t *TraceLogger) Log(step string, format string, args ...any) {
	if t == nil || !t.enabled || t.writer == nil {
		return
	}
	message := fmt.Sprintf(format, args...)
	fmt.Fprintf(t.writer, "[trace] %s: %s\n", step, message)
}

// formatTraceData 根据 trace payload 类型生成可读的一行日志内容。
func formatTraceData(data any) string {
	switch value := data.(type) {
	case ChatLoopStartTrace:
		return fmt.Sprintf("endpoint=%s model=%s think=%v tools=%d", value.Endpoint, value.Model, value.Think, value.Tools)
	case ChatLoopExitTrace:
		return fmt.Sprintf("command=%q", value.Command)
	case TurnInputTrace:
		return fmt.Sprintf("message=%q history_messages=%d", value.Message, value.HistoryMessages)
	case ModelRequestTrace:
		return fmt.Sprintf("phase=%s tool_round=%d request=%s", value.Phase, value.ToolRound, formatTraceJSON(value.Request))
	case ModelResponseTrace:
		response := struct {
			Content   string            `json:"content"`
			ToolCalls []ollama.ToolCall `json:"tool_calls,omitempty"`
		}{
			Content:   value.Content,
			ToolCalls: value.ToolCalls,
		}
		return fmt.Sprintf("phase=%s tool_round=%d response=%s", value.Phase, value.ToolRound, formatTraceJSON(response))
	case ToolCallTrace:
		return fmt.Sprintf("name=%s arguments=%v", value.Name, value.Arguments)
	case ToolResultTrace:
		return fmt.Sprintf("name=%s result=%s", value.Name, value.Result)
	case ToolErrorTrace:
		return fmt.Sprintf("name=%s error=%v", value.Name, value.Error)
	case PlannerRequestTrace:
		return fmt.Sprintf("message=%q message_chars=%d", value.Message, value.MessageChars)
	case PlannerResponseTrace:
		return fmt.Sprintf("goal=%q steps=%d", value.Goal, value.Steps)
	case ExecutorStartTrace:
		return fmt.Sprintf("steps=%d", value.Steps)
	case ExecutorStepTrace:
		return fmt.Sprintf("index=%d task=%q tool_hint=%q", value.Index, value.Task, value.ToolHint)
	case ExecutorFinishTrace:
		return fmt.Sprintf("content_chars=%d", value.ContentChars)
	case FinalAnswerTrace:
		return fmt.Sprintf("content_chars=%d history_messages=%d", value.ContentChars, value.HistoryMessages)
	default:
		return fmt.Sprintf("%v", data)
	}
}

// formatTraceJSON 将复杂 trace payload 格式化成缩进 JSON。
func formatTraceJSON(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}
