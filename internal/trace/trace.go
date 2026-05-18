package trace

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"mini-agent-runtime/internal/ollama"
)

// TraceEventName 表示 trace 事件的稳定名称。
type TraceEventName string

const (
	TraceRunStart          TraceEventName = "run_start"
	TraceRunFinish         TraceEventName = "run_finish"
	TraceStepStart         TraceEventName = "step_start"
	TraceStepFinish        TraceEventName = "step_finish"
	TraceObservation       TraceEventName = "observation"
	TraceChatLoopStart     TraceEventName = "chat_loop_start"
	TraceChatLoopExit      TraceEventName = "chat_loop_exit"
	TraceTurnInput         TraceEventName = "turn_input"
	TraceModelRequest      TraceEventName = "model_request"
	TraceModelResponse     TraceEventName = "model_response"
	TraceToolCall          TraceEventName = "tool_call"
	TraceToolResult        TraceEventName = "tool_result"
	TraceToolError         TraceEventName = "tool_error"
	TraceApprovalRequested TraceEventName = "approval_requested"
	TraceApprovalDecided   TraceEventName = "approval_decided"
	TraceApprovalBypassed  TraceEventName = "approval_bypassed"
	TracePlannerRequest    TraceEventName = "planner_request"
	TracePlannerResponse   TraceEventName = "planner_response"
	TraceExecutorStart     TraceEventName = "executor_start"
	TraceExecutorStep      TraceEventName = "executor_step"
	TraceExecutorFinish    TraceEventName = "executor_finish"
	TraceFinalAnswer       TraceEventName = "final_answer"
)

// TraceContext 表示 trace 事件所属的 run、step 和父 step 关联信息。
type TraceContext struct {
	RunID        string
	StepID       string
	ParentStepID string
}

// TraceEvent 表示发送给 trace sink 的结构化事件。
type TraceEvent struct {
	Name         TraceEventName
	RunID        string
	StepID       string
	ParentStepID string
	Data         any
}

// TraceSink 定义 trace 事件输出端的统一接口。
type TraceSink interface {
	Emit(event TraceEvent)
}

// TraceHooks 保存 trace sink 和当前上下文，并向业务流程暴露稳定 hook 方法。
type TraceHooks struct {
	sink    TraceSink
	context TraceContext
}

// NewTraceHooks 创建 trace hook 集合，用于把关键运行节点事件统一发送到 sink。
func NewTraceHooks(sink TraceSink) *TraceHooks {
	return &TraceHooks{sink: sink}
}

// WithContext 创建带有 run/step 上下文的 hook 视图，复用同一个 sink 输出事件。
func (h *TraceHooks) WithContext(context TraceContext) *TraceHooks {
	if h == nil {
		return &TraceHooks{context: context}
	}
	return &TraceHooks{sink: h.sink, context: context}
}

// emit 在 hook 可用时发送 trace 事件，空 hook 会被安全忽略。
func (h *TraceHooks) emit(name TraceEventName, data any) {
	if h == nil || h.sink == nil {
		return
	}
	h.sink.Emit(TraceEvent{
		Name:         name,
		RunID:        h.context.RunID,
		StepID:       h.context.StepID,
		ParentStepID: h.context.ParentStepID,
		Data:         data,
	})
}

// RunStartTrace 表示一次 agent run 开始时的摘要信息。
type RunStartTrace struct {
	Mode       string `json:"mode"`
	InputChars int    `json:"input_chars"`
}

// RunFinishTrace 表示一次 agent run 结束时的摘要信息。
type RunFinishTrace struct {
	Status      string `json:"status"`
	OutputChars int    `json:"output_chars"`
	Error       string `json:"error,omitempty"`
	DurationMs  int64  `json:"duration_ms"`
	Steps       int    `json:"steps"`
}

// StepStartTrace 表示一次 run 内部 step 开始时的摘要信息。
type StepStartTrace struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// StepFinishTrace 表示一次 run 内部 step 结束时的摘要信息。
type StepFinishTrace struct {
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

// ObservationTrace 表示 step 执行期间产生的观察结果。
type ObservationTrace struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ChatLoopStartTrace 表示 CLI 对话循环启动时的配置摘要。
type ChatLoopStartTrace struct {
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
	Think    bool   `json:"think"`
	Tools    int    `json:"tools"`
}

// ChatLoopExitTrace 表示 CLI 对话循环退出事件。
type ChatLoopExitTrace struct {
	Command string `json:"command"`
}

// TurnInputTrace 表示单轮用户输入事件。
type TurnInputTrace struct {
	Message         string `json:"message"`
	HistoryMessages int    `json:"history_messages"`
}

// ModelRequestTrace 表示发送给模型的完整请求。
type ModelRequestTrace struct {
	ToolRound int                `json:"tool_round"`
	Phase     string             `json:"phase"`
	Request   ollama.ChatRequest `json:"request"`
}

// ModelResponseTrace 表示模型返回的文本和工具调用。
type ModelResponseTrace struct {
	ToolRound int               `json:"tool_round"`
	Phase     string            `json:"phase"`
	Content   string            `json:"content"`
	ToolCalls []ollama.ToolCall `json:"tool_calls,omitempty"`
}

// ToolCallTrace 表示即将执行的工具调用。
type ToolCallTrace struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// ToolResultTrace 表示工具调用成功后的返回结果。
type ToolResultTrace struct {
	Name   string `json:"name"`
	Result string `json:"result"`
}

// ToolErrorTrace 表示工具调用失败事件。
type ToolErrorTrace struct {
	Name  string `json:"name"`
	Error error  `json:"error"`
}

// ApprovalRequestedTrace 表示高风险工具调用提交审批时的 trace 信息。
type ApprovalRequestedTrace struct {
	RequestID string `json:"request_id"`
	ToolName  string `json:"tool_name"`
	RiskLevel string `json:"risk_level"`
	Reason    string `json:"reason,omitempty"`
}

// ApprovalDecidedTrace 表示审批请求得到决策后的 trace 信息。
type ApprovalDecidedTrace struct {
	RequestID string `json:"request_id"`
	ToolName  string `json:"tool_name"`
	RiskLevel string `json:"risk_level"`
	Decision  string `json:"decision"`
	Approver  string `json:"approver,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// ApprovalBypassedTrace 表示工具调用不需要审批时的 trace 信息。
type ApprovalBypassedTrace struct {
	RequestID string `json:"request_id"`
	ToolName  string `json:"tool_name"`
	RiskLevel string `json:"risk_level"`
	Reason    string `json:"reason,omitempty"`
}

// PlannerRequestTrace 表示 planner 阶段收到的用户目标。
type PlannerRequestTrace struct {
	Message      string `json:"message"`
	MessageChars int    `json:"message_chars"`
}

// PlannerResponseTrace 表示 planner 阶段生成计划后的摘要。
type PlannerResponseTrace struct {
	Goal  string `json:"goal"`
	Steps int    `json:"steps"`
}

// ExecutorStartTrace 表示 executor 阶段启动事件。
type ExecutorStartTrace struct {
	Steps int `json:"steps"`
}

// ExecutorStepTrace 表示 executor 将要处理的计划步骤。
type ExecutorStepTrace struct {
	Index    int    `json:"index"`
	Task     string `json:"task"`
	ToolHint string `json:"tool_hint,omitempty"`
}

// ExecutorFinishTrace 表示 executor 阶段完成事件。
type ExecutorFinishTrace struct {
	ContentChars int `json:"content_chars"`
}

// FinalAnswerTrace 表示一轮对话最终回答进入历史后的事件。
type FinalAnswerTrace struct {
	ContentChars    int `json:"content_chars"`
	HistoryMessages int `json:"history_messages"`
}

// RunStart 记录一次 agent run 开始事件。
func (h *TraceHooks) RunStart(data RunStartTrace) {
	h.emit(TraceRunStart, data)
}

// RunFinish 记录一次 agent run 完成事件。
func (h *TraceHooks) RunFinish(data RunFinishTrace) {
	h.emit(TraceRunFinish, data)
}

// StepStart 记录一次 run 内部 step 开始事件。
func (h *TraceHooks) StepStart(data StepStartTrace) {
	h.emit(TraceStepStart, data)
}

// StepFinish 记录一次 run 内部 step 完成事件。
func (h *TraceHooks) StepFinish(data StepFinishTrace) {
	h.emit(TraceStepFinish, data)
}

// Observation 记录一次 step 内部观察结果事件。
func (h *TraceHooks) Observation(data ObservationTrace) {
	h.emit(TraceObservation, data)
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

// ApprovalRequested 记录高风险工具调用提交审批事件。
func (h *TraceHooks) ApprovalRequested(data ApprovalRequestedTrace) {
	h.emit(TraceApprovalRequested, data)
}

// ApprovalDecided 记录审批请求得到决策事件。
func (h *TraceHooks) ApprovalDecided(data ApprovalDecidedTrace) {
	h.emit(TraceApprovalDecided, data)
}

// ApprovalBypassed 记录工具调用不需要审批事件。
func (h *TraceHooks) ApprovalBypassed(data ApprovalBypassedTrace) {
	h.emit(TraceApprovalBypassed, data)
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

// TraceLogger 把 trace 事件格式化为人类可读文本。
type TraceLogger struct {
	enabled bool
	writer  io.Writer
}

// NewTraceLogger 创建把 trace 事件写入文本流的 logger。
func NewTraceLogger(enabled bool, writer io.Writer) *TraceLogger {
	return &TraceLogger{enabled: enabled, writer: writer}
}

// Emit 按统一格式输出单个 trace 事件。
func (t *TraceLogger) Emit(event TraceEvent) {
	if t == nil || !t.enabled || t.writer == nil {
		return
	}
	_, _ = fmt.Fprintf(t.writer, "[trace] %s: %s\n", event.Name, formatTraceData(event.Data))
}

// Log 保留兼容旧调用的格式化 trace 日志输出能力。
func (t *TraceLogger) Log(step string, format string, args ...any) {
	if t == nil || !t.enabled || t.writer == nil {
		return
	}
	message := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(t.writer, "[trace] %s: %s\n", step, message)
}

// MultiSink 把同一份 trace 事件广播到多个 sink。
type MultiSink struct {
	sinks []TraceSink
}

// NewMultiSink 创建会顺序广播事件的组合 sink，nil sink 会被忽略。
func NewMultiSink(sinks ...TraceSink) *MultiSink {
	filtered := make([]TraceSink, 0, len(sinks))
	for _, sink := range sinks {
		if sink != nil {
			filtered = append(filtered, sink)
		}
	}
	return &MultiSink{sinks: filtered}
}

// Emit 把 trace 事件发送给组合 sink 中的每一个输出端。
func (m *MultiSink) Emit(event TraceEvent) {
	if m == nil {
		return
	}
	for _, sink := range m.sinks {
		sink.Emit(event)
	}
}

// TraceJSONLLogger 把 trace 事件按 JSON Lines 格式写入输出流。
type TraceJSONLLogger struct {
	enabled bool
	writer  io.Writer
	clock   func() time.Time
}

// NewTraceJSONLLogger 创建默认使用当前时间的 JSONL trace sink。
func NewTraceJSONLLogger(enabled bool, writer io.Writer) *TraceJSONLLogger {
	return NewTraceJSONLLoggerWithClock(enabled, writer, time.Now)
}

// NewTraceJSONLLoggerWithClock 创建可注入时间来源的 JSONL trace sink，便于测试稳定断言。
func NewTraceJSONLLoggerWithClock(enabled bool, writer io.Writer, clock func() time.Time) *TraceJSONLLogger {
	if clock == nil {
		clock = time.Now
	}
	return &TraceJSONLLogger{enabled: enabled, writer: writer, clock: clock}
}

// Emit 把单个 trace 事件序列化为一行 JSON。
func (l *TraceJSONLLogger) Emit(event TraceEvent) {
	if l == nil || !l.enabled || l.writer == nil {
		return
	}
	payload := map[string]any{
		"timestamp": l.clock().UTC().Format(time.RFC3339Nano),
		"name":      event.Name,
	}
	if event.RunID != "" {
		payload["run_id"] = event.RunID
	}
	if event.StepID != "" {
		payload["step_id"] = event.StepID
	}
	if event.ParentStepID != "" {
		payload["parent_step_id"] = event.ParentStepID
	}
	if event.Data != nil {
		payload["data"] = event.Data
	}
	data, err := json.Marshal(payload)
	if err != nil {
		_, _ = fmt.Fprintf(l.writer, `{"name":"trace_marshal_error","error":%q}`+"\n", err.Error())
		return
	}
	_, _ = fmt.Fprintf(l.writer, "%s\n", data)
}

// formatTraceData 根据 trace payload 类型生成可读的一行日志内容。
func formatTraceData(data any) string {
	switch value := data.(type) {
	case RunStartTrace:
		return fmt.Sprintf("mode=%s input_chars=%d", value.Mode, value.InputChars)
	case RunFinishTrace:
		return fmt.Sprintf("status=%s output_chars=%d error=%q duration_ms=%d steps=%d", value.Status, value.OutputChars, value.Error, value.DurationMs, value.Steps)
	case StepStartTrace:
		return fmt.Sprintf("type=%s name=%s", value.Type, value.Name)
	case StepFinishTrace:
		return fmt.Sprintf("status=%s error=%q duration_ms=%d", value.Status, value.Error, value.DurationMs)
	case ObservationTrace:
		return fmt.Sprintf("type=%s name=%s content=%q error=%q", value.Type, value.Name, value.Content, value.Error)
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
	case ApprovalRequestedTrace:
		return fmt.Sprintf("request_id=%s tool=%s risk=%s reason=%q", value.RequestID, value.ToolName, value.RiskLevel, value.Reason)
	case ApprovalDecidedTrace:
		return fmt.Sprintf("request_id=%s tool=%s risk=%s decision=%s approver=%s reason=%q", value.RequestID, value.ToolName, value.RiskLevel, value.Decision, value.Approver, value.Reason)
	case ApprovalBypassedTrace:
		return fmt.Sprintf("request_id=%s tool=%s risk=%s reason=%q", value.RequestID, value.ToolName, value.RiskLevel, value.Reason)
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
