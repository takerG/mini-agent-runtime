package agent

import "testing"

// TestNewModeRunnerCreatesDedicatedRunner 验证模式分发被收敛到 runner 工厂，而不是散落在 CLI 循环中。
func TestNewModeRunnerCreatesDedicatedRunner(t *testing.T) {
	tests := []struct {
		name string
		mode Mode
		want any
	}{
		{name: "chat", mode: ModeChat, want: (*ChatRunner)(nil)},
		{name: "plan", mode: ModePlan, want: (*PlannerRunner)(nil)},
		{name: "strict-plan", mode: ModeStrictPlan, want: (*StrictPlannerRunner)(nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner, err := NewModeRunner(RunnerOptions{Mode: tt.mode})
			if err != nil {
				t.Fatalf("NewModeRunner returned error: %v", err)
			}

			switch tt.want.(type) {
			case *ChatRunner:
				if _, ok := runner.(*ChatRunner); !ok {
					t.Fatalf("runner type = %T, want *ChatRunner", runner)
				}
			case *PlannerRunner:
				if _, ok := runner.(*PlannerRunner); !ok {
					t.Fatalf("runner type = %T, want *PlannerRunner", runner)
				}
			case *StrictPlannerRunner:
				if _, ok := runner.(*StrictPlannerRunner); !ok {
					t.Fatalf("runner type = %T, want *StrictPlannerRunner", runner)
				}
			}
		})
	}
}

// TestNewModeRunnerRejectsUnknownMode 验证未知模式会在 runner 装配阶段失败，避免 CLI 运行到中途才暴露错误。
func TestNewModeRunnerRejectsUnknownMode(t *testing.T) {
	_, err := NewModeRunner(RunnerOptions{Mode: Mode("unknown")})
	if err == nil {
		t.Fatal("NewModeRunner returned nil error, want unknown mode error")
	}
}
