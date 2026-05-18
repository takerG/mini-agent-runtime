package lifecycle

import (
	"fmt"
	"time"
)

// Status 表示 run 或 step 当前所处的生命周期状态。
type Status string

const (
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

// StepType 表示一次 run 内部可观测步骤的类型。
type StepType string

const (
	StepTypeModelRequest StepType = "model_request"
	StepTypePlanner      StepType = "planner"
	StepTypeExecutor     StepType = "executor"
	StepTypeToolCall     StepType = "tool_call"
	StepTypeSummary      StepType = "summary"
)

// ObservationType 表示 step 执行过程中产生的观察结果类型。
type ObservationType string

const (
	ObservationTypeModelResponse    ObservationType = "model_response"
	ObservationTypeToolResult       ObservationType = "tool_result"
	ObservationTypeToolError        ObservationType = "tool_error"
	ObservationTypeApprovalRequest  ObservationType = "approval_request"
	ObservationTypeApprovalDecision ObservationType = "approval_decision"
	ObservationTypeMemoryContext    ObservationType = "memory_context"
	ObservationTypeFinalAnswer      ObservationType = "final_answer"
)

// Clock 定义 lifecycle 使用的时间来源，便于测试中注入稳定时钟。
type Clock func() time.Time

// IDGenerator 定义 run 和 step 的 ID 生成函数，kind 通常是 run 或 step。
type IDGenerator func(kind string, sequence int) string

// FactoryOptions 描述创建 lifecycle factory 时可以注入的基础能力。
type FactoryOptions struct {
	Clock       Clock
	IDGenerator IDGenerator
}

// Factory 负责为每一轮 agent 请求创建独立的 lifecycle recorder。
type Factory struct {
	clock       Clock
	idGenerator IDGenerator
	nextRun     int
}

// Run 表示一次用户请求从进入 runtime 到最终完成的完整生命周期记录。
type Run struct {
	ID         string
	Mode       string
	Input      string
	Status     Status
	StartedAt  time.Time
	FinishedAt time.Time
	Duration   time.Duration
	Steps      []Step
	Result     Result
}

// Step 表示一次 run 内部的单个可观测执行步骤。
type Step struct {
	ID           string
	ParentID     string
	Type         StepType
	Name         string
	Status       Status
	StartedAt    time.Time
	FinishedAt   time.Time
	Duration     time.Duration
	Metadata     map[string]any
	Observations []Observation
}

// Observation 表示 step 执行期间产生的模型返回、工具结果或错误等观察信息。
type Observation struct {
	StepID    string
	Type      ObservationType
	Name      string
	Content   string
	Error     string
	CreatedAt time.Time
}

// Result 表示 run 完成后面向调用方的最终结果。
type Result struct {
	Output string
	Error  string
}

// Recorder 负责记录单次 run 内部的 step、observation 和最终结果。
type Recorder struct {
	clock       Clock
	idGenerator IDGenerator
	run         Run
	nextStep    int
}

// NewFactory 创建 lifecycle factory，并为未传入的时钟和 ID 生成器补齐默认值。
func NewFactory(options FactoryOptions) *Factory {
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	idGenerator := options.IDGenerator
	if idGenerator == nil {
		idGenerator = defaultIDGenerator
	}
	return &Factory{clock: clock, idGenerator: idGenerator}
}

// Start 为一次用户请求创建新的 recorder，并立即标记 run 为 running。
func (f *Factory) Start(mode string, input string) *Recorder {
	if f == nil {
		f = NewFactory(FactoryOptions{})
	}
	f.nextRun++
	run := Run{
		ID:        f.idGenerator("run", f.nextRun),
		Mode:      mode,
		Input:     input,
		Status:    StatusRunning,
		StartedAt: f.clock(),
	}
	return &Recorder{clock: f.clock, idGenerator: f.idGenerator, run: run}
}

// RunID 返回当前 recorder 所属 run 的稳定 ID。
func (r *Recorder) RunID() string {
	if r == nil {
		return ""
	}
	return r.run.ID
}

// StartStep 创建新的执行步骤，并把步骤追加到当前 run。
func (r *Recorder) StartStep(parentID string, stepType StepType, name string, metadata map[string]any) Step {
	if r == nil {
		return Step{}
	}
	r.nextStep++
	step := Step{
		ID:        r.idGenerator("step", r.nextStep),
		ParentID:  parentID,
		Type:      stepType,
		Name:      name,
		Status:    StatusRunning,
		StartedAt: r.clock(),
		Metadata:  copyMetadata(metadata),
	}
	r.run.Steps = append(r.run.Steps, step)
	return step
}

// AddObservation 把 step 执行期间产生的观察结果追加到对应步骤上。
func (r *Recorder) AddObservation(stepID string, observationType ObservationType, name string, content string, err error) {
	if r == nil {
		return
	}
	observation := Observation{
		StepID:    stepID,
		Type:      observationType,
		Name:      name,
		Content:   content,
		CreatedAt: r.clock(),
	}
	if err != nil {
		observation.Error = err.Error()
	}
	index := r.stepIndex(stepID)
	if index < 0 {
		return
	}
	r.run.Steps[index].Observations = append(r.run.Steps[index].Observations, observation)
}

// FinishStep 标记指定 step 已结束，并根据 err 决定成功或失败状态。
func (r *Recorder) FinishStep(stepID string, err error) {
	if r == nil {
		return
	}
	index := r.stepIndex(stepID)
	if index < 0 {
		return
	}
	finishedAt := r.clock()
	step := &r.run.Steps[index]
	step.FinishedAt = finishedAt
	step.Duration = finishedAt.Sub(step.StartedAt)
	if err != nil {
		step.Status = StatusFailed
		return
	}
	step.Status = StatusSucceeded
}

// Finish 标记当前 run 已完成，并返回一份生命周期快照。
func (r *Recorder) Finish(output string, err error) Run {
	if r == nil {
		return Run{}
	}
	finishedAt := r.clock()
	r.run.FinishedAt = finishedAt
	r.run.Duration = finishedAt.Sub(r.run.StartedAt)
	r.run.Result.Output = output
	if err != nil {
		r.run.Status = StatusFailed
		r.run.Result.Error = err.Error()
		return r.Run()
	}
	r.run.Status = StatusSucceeded
	return r.Run()
}

// Run 返回当前 lifecycle 的不可变快照，避免调用方修改 recorder 内部状态。
func (r *Recorder) Run() Run {
	if r == nil {
		return Run{}
	}
	snapshot := r.run
	snapshot.Steps = make([]Step, len(r.run.Steps))
	for i, step := range r.run.Steps {
		snapshot.Steps[i] = step
		snapshot.Steps[i].Metadata = copyMetadata(step.Metadata)
		snapshot.Steps[i].Observations = append([]Observation(nil), step.Observations...)
	}
	return snapshot
}

// stepIndex 返回指定 step 在 run 步骤列表中的位置，找不到时返回 -1。
func (r *Recorder) stepIndex(stepID string) int {
	for i, step := range r.run.Steps {
		if step.ID == stepID {
			return i
		}
	}
	return -1
}

// defaultIDGenerator 使用固定前缀和递增序号生成便于阅读与测试的 ID。
func defaultIDGenerator(kind string, sequence int) string {
	return fmt.Sprintf("%s-%06d", kind, sequence)
}

// copyMetadata 复制 step 元数据 map，避免外部修改影响 lifecycle 快照。
func copyMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	copied := make(map[string]any, len(metadata))
	for key, value := range metadata {
		copied[key] = value
	}
	return copied
}
