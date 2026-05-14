package agent

import (
	"mini-agent-runtime/internal/lifecycle"
	tracing "mini-agent-runtime/internal/trace"
)

// startLifecycleRun 创建单轮 agent run，并发出 run_start trace 事件。
func startLifecycleRun(traceHooks *tracing.TraceHooks, factory *lifecycle.Factory, mode Mode, input string) *lifecycle.Recorder {
	if factory == nil {
		factory = lifecycle.NewFactory(lifecycle.FactoryOptions{})
	}
	recorder := factory.Start(string(mode), input)
	traceHooks.WithContext(runTraceContext(recorder)).RunStart(tracing.RunStartTrace{
		Mode:       string(mode),
		InputChars: len([]rune(input)),
	})
	return recorder
}

// finishLifecycleRun 结束单轮 agent run，并发出 run_finish trace 事件。
func finishLifecycleRun(traceHooks *tracing.TraceHooks, recorder *lifecycle.Recorder, output string, err error) lifecycle.Run {
	run := recorder.Finish(output, err)
	traceHooks.WithContext(runTraceContext(recorder)).RunFinish(tracing.RunFinishTrace{
		Status:      string(run.Status),
		OutputChars: len([]rune(output)),
		Error:       run.Result.Error,
		DurationMs:  run.Duration.Milliseconds(),
		Steps:       len(run.Steps),
	})
	return run
}

// startLifecycleStep 创建 run 内部 step，并返回带 step 上下文的 trace hooks。
func startLifecycleStep(traceHooks *tracing.TraceHooks, recorder *lifecycle.Recorder, parentID string, stepType lifecycle.StepType, name string, metadata map[string]any) (lifecycle.Step, *tracing.TraceHooks) {
	step := recorder.StartStep(parentID, stepType, name, metadata)
	stepTrace := traceHooks.WithContext(stepTraceContext(recorder, step))
	stepTrace.StepStart(tracing.StepStartTrace{Type: string(step.Type), Name: step.Name})
	return step, stepTrace
}

// finishLifecycleStep 结束 run 内部 step，并发出 step_finish trace 事件。
func finishLifecycleStep(stepTrace *tracing.TraceHooks, recorder *lifecycle.Recorder, step lifecycle.Step, err error) {
	recorder.FinishStep(step.ID, err)
	latest := lifecycleStepByID(recorder.Run(), step.ID)
	stepTrace.StepFinish(tracing.StepFinishTrace{
		Status:     string(latest.Status),
		Error:      errorString(err),
		DurationMs: latest.Duration.Milliseconds(),
	})
}

// addLifecycleObservation 记录 step 观察结果，并发出 observation trace 事件。
func addLifecycleObservation(stepTrace *tracing.TraceHooks, recorder *lifecycle.Recorder, step lifecycle.Step, observationType lifecycle.ObservationType, name string, content string, err error) {
	recorder.AddObservation(step.ID, observationType, name, content, err)
	stepTrace.Observation(tracing.ObservationTrace{
		Type:    string(observationType),
		Name:    name,
		Content: content,
		Error:   errorString(err),
	})
}

// runTraceContext 根据 recorder 构造 run 级 trace 上下文。
func runTraceContext(recorder *lifecycle.Recorder) tracing.TraceContext {
	if recorder == nil {
		return tracing.TraceContext{}
	}
	return tracing.TraceContext{RunID: recorder.RunID()}
}

// stepTraceContext 根据 recorder 和 step 构造 step 级 trace 上下文。
func stepTraceContext(recorder *lifecycle.Recorder, step lifecycle.Step) tracing.TraceContext {
	return tracing.TraceContext{
		RunID:        recorder.RunID(),
		StepID:       step.ID,
		ParentStepID: step.ParentID,
	}
}

// lifecycleStepByID 从 run 快照中查找指定 step，找不到时返回空 step。
func lifecycleStepByID(run lifecycle.Run, stepID string) lifecycle.Step {
	for _, step := range run.Steps {
		if step.ID == stepID {
			return step
		}
	}
	return lifecycle.Step{}
}

// errorString 把可选错误转换为 trace 里更稳定的字符串。
func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
