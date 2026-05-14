package agent

import (
	"context"
	"fmt"
	"io"

	apperrors "mini-agent-runtime/internal/errors"
	"mini-agent-runtime/internal/lifecycle"
	"mini-agent-runtime/internal/memory"
	tracing "mini-agent-runtime/internal/trace"
)

// turnExecutor 定义单轮 agent 在具体模式下的核心执行接口。
type turnExecutor interface {
	ExecuteTurn(ctx context.Context, options turnExecutionOptions) (string, error)
}

// turnExecutionOptions 描述具体模式执行器需要读取的单轮上下文。
type turnExecutionOptions struct {
	Session     *Session
	UserMessage string
	Recorder    *lifecycle.Recorder
}

// turnCoordinatorOptions 描述统一单轮生命周期协调器需要的依赖。
type turnCoordinatorOptions struct {
	Mode                 Mode
	Trace                *tracing.TraceHooks
	Lifecycle            *lifecycle.Factory
	Session              *Session
	Memory               *memory.Manager
	MemoryQuery          memory.Query
	UserMessage          string
	Stdout               io.Writer
	PrintTrailingNewline bool
	Executor             turnExecutor
}

// runAgentTurn 统一处理单轮开始、用户输入记录、模式执行、memory 写入、最终 trace 和 run 收尾。
func runAgentTurn(ctx context.Context, options turnCoordinatorOptions) (result TurnResult, err error) {
	recorder := startLifecycleRun(options.Trace, options.Lifecycle, options.Mode, options.UserMessage)
	defer func() {
		finishLifecycleRun(options.Trace, recorder, result.AssistantMessage, err)
	}()

	historyMessages := 1
	if options.Session != nil {
		options.Session.AppendUserMessage(options.UserMessage)
		historyMessages = len(options.Session.History())
	}
	options.Trace.WithContext(runTraceContext(recorder)).TurnInput(tracing.TurnInputTrace{
		Message:         options.UserMessage,
		HistoryMessages: historyMessages,
	})

	if options.Executor == nil {
		return TurnResult{}, apperrors.New(apperrors.NodeAgentLoop, apperrors.CodeUnknown, "missing turn executor")
	}
	assistantMessage, err := options.Executor.ExecuteTurn(ctx, turnExecutionOptions{
		Session:     options.Session,
		UserMessage: options.UserMessage,
		Recorder:    recorder,
	})
	if err != nil {
		return TurnResult{}, err
	}
	if options.PrintTrailingNewline {
		_, _ = fmt.Fprintln(options.Stdout)
	}
	if err := commitTurnMemory(ctx, options, assistantMessage); err != nil {
		return TurnResult{}, err
	}

	historyMessages = 0
	if options.Session != nil {
		historyMessages = len(options.Session.History())
	}
	options.Trace.WithContext(runTraceContext(recorder)).FinalAnswer(tracing.FinalAnswerTrace{
		ContentChars:    len([]rune(assistantMessage)),
		HistoryMessages: historyMessages,
	})
	return TurnResult{AssistantMessage: assistantMessage}, nil
}

// commitTurnMemory 将单轮最终结果写入 session 或 runtime memory。
func commitTurnMemory(ctx context.Context, options turnCoordinatorOptions, assistantMessage string) error {
	if options.Session != nil {
		if assistantMessage != "" {
			options.Session.AppendAssistantMessage(assistantMessage)
		}
		return options.Session.CommitTurn(ctx, options.UserMessage, assistantMessage)
	}
	if options.Memory == nil {
		return nil
	}
	if err := options.Memory.AppendTurn(ctx, defaultMemoryQuery(options.MemoryQuery), memory.Turn{User: options.UserMessage, Assistant: assistantMessage}); err != nil {
		return apperrors.Wrap(apperrors.NodeAgentLoop, apperrors.CodeUnknown, err, "append runtime memory turn")
	}
	return nil
}
