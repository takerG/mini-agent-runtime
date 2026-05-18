package lifecycle

import (
	"errors"
	"testing"
	"time"
)

// TestRecorderTracksRunStepsObservationsAndResult 验证 recorder 会完整记录一次 run 的步骤、观察结果和最终状态。
func TestRecorderTracksRunStepsObservationsAndResult(t *testing.T) {
	clock := newStepClock(time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC), time.Second)
	factory := NewFactory(FactoryOptions{Clock: clock.Now})
	recorder := factory.Start("strict-plan", "计算 23 * 19")

	step := recorder.StartStep("", StepTypeToolCall, "calculator", map[string]any{"tool": "calculator"})
	recorder.AddObservation(step.ID, ObservationTypeToolResult, "calculator", "437", nil)
	recorder.FinishStep(step.ID, nil)
	run := recorder.Finish("计算结果是 437", nil)

	if got, want := run.ID, "run-000001"; got != want {
		t.Fatalf("run id = %q, want %q", got, want)
	}
	if got, want := run.Status, StatusSucceeded; got != want {
		t.Fatalf("run status = %q, want %q", got, want)
	}
	if got, want := run.Result.Output, "计算结果是 437"; got != want {
		t.Fatalf("run output = %q, want %q", got, want)
	}
	if got, want := len(run.Steps), 1; got != want {
		t.Fatalf("step count = %d, want %d", got, want)
	}
	if got, want := run.Steps[0].ID, "step-000001"; got != want {
		t.Fatalf("step id = %q, want %q", got, want)
	}
	if got, want := run.Steps[0].Status, StatusSucceeded; got != want {
		t.Fatalf("step status = %q, want %q", got, want)
	}
	if got, want := run.Steps[0].Observations[0].Content, "437"; got != want {
		t.Fatalf("observation content = %q, want %q", got, want)
	}
	if run.Duration <= 0 {
		t.Fatalf("run duration = %s, want positive", run.Duration)
	}
}

// TestRecorderTracksApprovalObservations 验证 lifecycle 可以记录审批请求和审批决策 observation。
func TestRecorderTracksApprovalObservations(t *testing.T) {
	factory := NewFactory(FactoryOptions{})
	recorder := factory.Start("chat", "执行高危操作")
	step := recorder.StartStep("", StepTypeToolCall, "dangerous_operation", nil)

	recorder.AddObservation(step.ID, ObservationTypeApprovalRequest, "dangerous_operation", "approval requested", nil)
	recorder.AddObservation(step.ID, ObservationTypeApprovalDecision, "dangerous_operation", "approval approved", nil)
	run := recorder.Finish("已完成", nil)

	if got, want := run.Steps[0].Observations[0].Type, ObservationTypeApprovalRequest; got != want {
		t.Fatalf("first observation type = %q, want %q", got, want)
	}
	if got, want := run.Steps[0].Observations[1].Type, ObservationTypeApprovalDecision; got != want {
		t.Fatalf("second observation type = %q, want %q", got, want)
	}
}

// TestRecorderMarksFailures 验证 step 或 run 失败时会记录错误文本和失败状态。
func TestRecorderMarksFailures(t *testing.T) {
	factory := NewFactory(FactoryOptions{})
	recorder := factory.Start("chat", "调用失败工具")
	step := recorder.StartStep("", StepTypeToolCall, "missing_tool", nil)
	err := errors.New("unknown tool")

	recorder.AddObservation(step.ID, ObservationTypeToolError, "missing_tool", "", err)
	recorder.FinishStep(step.ID, err)
	run := recorder.Finish("", err)

	if got, want := run.Status, StatusFailed; got != want {
		t.Fatalf("run status = %q, want %q", got, want)
	}
	if got, want := run.Steps[0].Status, StatusFailed; got != want {
		t.Fatalf("step status = %q, want %q", got, want)
	}
	if got, want := run.Result.Error, "unknown tool"; got != want {
		t.Fatalf("run error = %q, want %q", got, want)
	}
	if got, want := run.Steps[0].Observations[0].Error, "unknown tool"; got != want {
		t.Fatalf("observation error = %q, want %q", got, want)
	}
}

type stepClock struct {
	current time.Time
	step    time.Duration
}

// newStepClock 创建每次读取都会前进固定时间的测试时钟。
func newStepClock(start time.Time, step time.Duration) *stepClock {
	return &stepClock{current: start.Add(-step), step: step}
}

// Now 返回测试时钟的下一个时间点。
func (c *stepClock) Now() time.Time {
	c.current = c.current.Add(c.step)
	return c.current
}
