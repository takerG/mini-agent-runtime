package eval

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mini-agent-runtime/internal/agent"
	apperrors "mini-agent-runtime/internal/errors"
	"mini-agent-runtime/internal/lifecycle"
	"mini-agent-runtime/internal/memory"
	modelclient "mini-agent-runtime/internal/model"
	"mini-agent-runtime/internal/tools"
	tracing "mini-agent-runtime/internal/trace"
)

// RunnerOptions 描述 eval runner 运行 suite 时需要的模型、工具和输出配置。
type RunnerOptions struct {
	Endpoint   string
	Model      string
	Think      bool
	Client     *http.Client
	Output     io.Writer
	TraceSinks []tracing.TraceSink
	Tools      *tools.ToolRegistry
	ToolPolicy tools.ExecutionPolicy
}

// Runner 负责执行 eval suite，并把每条用例的实际行为与期望进行比较。
type Runner struct {
	options RunnerOptions
}

// Report 表示一次 eval suite 执行后的汇总结果。
type Report struct {
	Name    string
	Results []CaseResult
	Passed  int
	Failed  int
}

// CaseResult 表示单条用例在某个 mode 下的执行和判定结果。
type CaseResult struct {
	CaseName   string
	Mode       agent.Mode
	Passed     bool
	Answer     string
	Error      string
	Stdout     string
	ToolCalls  []ObservedToolCall
	ToolResults []ObservedToolResult
	ToolErrors []ObservedToolError
	Observations []ObservedObservation
	ApprovalRequests []ObservedApprovalRequest
	ApprovalDecisions []ObservedApprovalDecision
	Failures   []string
	Duration   time.Duration
}

// ObservedToolCall 表示 eval runner 从 trace 中观察到的工具调用。
type ObservedToolCall struct {
	Name      string
	Arguments map[string]any
}

// ObservedToolResult 表示 eval runner 从 trace 中观察到的工具结果。
type ObservedToolResult struct {
	Name   string
	Result string
}

// ObservedToolError 表示 eval runner 从 trace 中观察到的工具错误。
type ObservedToolError struct {
	Name  string
	Error string
}

// ObservedObservation 表示 eval runner 从 trace 中观察到的 lifecycle observation。
type ObservedObservation struct {
	Type    string
	Name    string
	Content string
	Error   string
}

// ObservedApprovalRequest 表示 eval runner 从 trace 中观察到的审批请求。
type ObservedApprovalRequest struct {
	ToolName  string
	RiskLevel string
	Reason    string
}

// ObservedApprovalDecision 表示 eval runner 从 trace 中观察到的审批决策。
type ObservedApprovalDecision struct {
	ToolName  string
	RiskLevel string
	Decision  string
	Approver  string
	Reason    string
}

// NewRunner 创建 eval runner。
func NewRunner(options RunnerOptions) *Runner {
	if options.Client == nil {
		options.Client = http.DefaultClient
	}
	if options.Output == nil {
		options.Output = io.Discard
	}
	if options.Tools == nil {
		options.Tools = tools.NewDefaultToolRegistry(time.Now)
	}
	if options.ToolPolicy.MaxAttempts == 0 {
		options.ToolPolicy = tools.DefaultExecutionPolicy()
	}
	return &Runner{options: options}
}

// RunSuite 执行 suite 中的所有用例和目标 mode，并返回完整报告。
func (r *Runner) RunSuite(ctx context.Context, suite Suite) (Report, error) {
	if err := suite.Validate(); err != nil {
		return Report{}, err
	}
	report := Report{Name: suite.Name}
	for _, evalCase := range suite.Cases {
		for _, mode := range evalCase.TargetModes() {
			result := r.runCaseMode(ctx, evalCase, mode)
			report.Results = append(report.Results, result)
			if result.Passed {
				report.Passed++
				continue
			}
			report.Failed++
		}
	}
	return report, nil
}

// WriteText 将 eval 报告以适合 CLI 阅读的格式写出。
func (r Report) WriteText(writer io.Writer) {
	if writer == nil {
		writer = io.Discard
	}
	_, _ = fmt.Fprintf(writer, "[eval] %s\n", r.Name)
	for _, result := range r.Results {
		status := "PASS"
		if !result.Passed {
			status = "FAIL"
		}
		_, _ = fmt.Fprintf(writer, "%s %s mode=%s duration=%s\n", status, result.CaseName, result.Mode, result.Duration.Round(time.Millisecond))
		for _, failure := range result.Failures {
			_, _ = fmt.Fprintf(writer, "  - %s\n", failure)
		}
	}
	_, _ = fmt.Fprintf(writer, "summary: %d passed, %d failed\n", r.Passed, r.Failed)
}

// FailureError 在报告存在失败用例时返回可交给 CLI 的错误。
func (r Report) FailureError() error {
	if r.Failed == 0 {
		return nil
	}
	return fmt.Errorf("eval suite %s failed: %d failed, %d passed", r.Name, r.Failed, r.Passed)
}

// runCaseMode 执行单条用例的单个 mode，并完成期望判定。
func (r *Runner) runCaseMode(ctx context.Context, evalCase Case, mode agent.Mode) CaseResult {
	startedAt := time.Now()
	stdout := &strings.Builder{}
	collector := &traceCollector{}
	traceHooks := tracing.NewTraceHooks(tracing.NewMultiSink(append([]tracing.TraceSink{collector}, r.options.TraceSinks...)...))
	modelClient := modelclient.NewClient(modelclient.Options{
		Endpoint: r.options.Endpoint,
		Model:    r.options.Model,
		Think:    r.options.Think,
		HTTP:     r.options.Client,
		Trace:    traceHooks,
	})
	memoryManager := memory.NewDefaultManager()
	memoryQuery := memory.Query{UserID: "eval", SessionID: safeSessionID(evalCase.Name, mode)}
	runner, err := agent.NewModeRunner(agent.RunnerOptions{
		Mode:        mode,
		ModelClient: modelClient,
		Tools:       r.options.Tools,
		ToolPolicy:  r.options.ToolPolicy,
		Trace:       traceHooks,
		Reporter:    apperrors.NewReporter(false, r.options.Output),
		Stdout:      stdout,
		Memory:      memoryManager,
		MemoryQuery: memoryQuery,
		Lifecycle:   lifecycle.NewFactory(lifecycle.FactoryOptions{}),
	})
	result := CaseResult{
		CaseName: evalCase.Name,
		Mode:     mode,
		Stdout:   stdout.String(),
	}
	if err == nil {
		session := agent.NewSession(agent.SessionOptions{Memory: memoryManager, MemoryQuery: memoryQuery})
		turnResult, runErr := runner.RunTurn(ctx, session, evalCase.Input)
		result.Answer = turnResult.AssistantMessage
		if runErr != nil {
			result.Error = runErr.Error()
		}
	} else {
		result.Error = err.Error()
	}
	result.Stdout = stdout.String()
	result.ToolCalls = collector.ToolCalls()
	result.ToolResults = collector.ToolResults()
	result.ToolErrors = collector.ToolErrors()
	result.Observations = collector.Observations()
	result.ApprovalRequests = collector.ApprovalRequests()
	result.ApprovalDecisions = collector.ApprovalDecisions()
	result.Duration = time.Since(startedAt)
	result.Failures = evaluateExpectation(evalCase.Expect, result)
	result.Passed = len(result.Failures) == 0
	return result
}

// evaluateExpectation 对单次执行结果进行断言，并返回所有失败原因。
func evaluateExpectation(expect Expectation, result CaseResult) []string {
	var failures []string
	failures = append(failures, evaluateFatalError(expect, result)...)
	failures = append(failures, evaluateAnswer(expect, result)...)
	failures = append(failures, evaluateToolCalls(expect, result)...)
	failures = append(failures, evaluateToolResults(expect, result)...)
	failures = append(failures, evaluateObservations(expect, result)...)
	failures = append(failures, evaluateApprovalRequests(expect, result)...)
	failures = append(failures, evaluateApprovalDecisions(expect, result)...)
	failures = append(failures, evaluateToolErrors(expect, result)...)
	return failures
}

// evaluateFatalError 检查 fatal error 是否符合预期。
func evaluateFatalError(expect Expectation, result CaseResult) []string {
	if expect.ExpectError {
		if result.Error == "" {
			return []string{"expected fatal error, got nil"}
		}
		return containsAll("fatal error", result.Error, expect.ErrorContains)
	}
	if result.Error != "" {
		return []string{fmt.Sprintf("unexpected fatal error: %s", result.Error)}
	}
	return nil
}

// evaluateAnswer 检查最终答案是否符合包含和排除断言。
func evaluateAnswer(expect Expectation, result CaseResult) []string {
	var failures []string
	failures = append(failures, containsAll("answer", result.Answer, expect.AnswerContains)...)
	for _, forbidden := range expect.AnswerNotContains {
		if strings.Contains(result.Answer, forbidden) {
			failures = append(failures, fmt.Sprintf("answer contains forbidden substring %q", forbidden))
		}
	}
	return failures
}

// evaluateToolCalls 检查工具调用数量、顺序、名称和参数。
func evaluateToolCalls(expect Expectation, result CaseResult) []string {
	var failures []string
	if expect.ToolCallCount != nil && len(result.ToolCalls) != *expect.ToolCallCount {
		failures = append(failures, fmt.Sprintf("tool call count = %d, want %d", len(result.ToolCalls), *expect.ToolCallCount))
	}
	if len(expect.ToolCalls) > len(result.ToolCalls) {
		failures = append(failures, fmt.Sprintf("tool calls = %d, want at least %d", len(result.ToolCalls), len(expect.ToolCalls)))
		return failures
	}
	for index, expected := range expect.ToolCalls {
		observed := result.ToolCalls[index]
		if expected.Name != "" && observed.Name != expected.Name {
			failures = append(failures, fmt.Sprintf("tool call[%d] name = %q, want %q", index, observed.Name, expected.Name))
		}
		for key, want := range expected.Arguments {
			got, ok := observed.Arguments[key]
			if !ok {
				failures = append(failures, fmt.Sprintf("tool call[%d] missing argument %q", index, key))
				continue
			}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				failures = append(failures, fmt.Sprintf("tool call[%d] argument %s = %v, want %v", index, key, got, want))
			}
		}
	}
	return failures
}

// evaluateToolResults 检查结构化工具结果的顺序、名称和内容。
func evaluateToolResults(expect Expectation, result CaseResult) []string {
	var failures []string
	if len(expect.ToolResults) > len(result.ToolResults) {
		failures = append(failures, fmt.Sprintf("tool results = %d, want at least %d", len(result.ToolResults), len(expect.ToolResults)))
		return failures
	}
	for index, expected := range expect.ToolResults {
		observed := result.ToolResults[index]
		failures = append(failures, evaluateToolResult(index, expected, observed)...)
	}
	return failures
}

// evaluateToolResult 检查单个结构化工具结果是否符合预期。
func evaluateToolResult(index int, expected ExpectedToolResult, observed ObservedToolResult) []string {
	var failures []string
	if expected.Name != "" && observed.Name != expected.Name {
		failures = append(failures, fmt.Sprintf("tool result[%d] name = %q, want %q", index, observed.Name, expected.Name))
	}
	failures = append(failures, containsAll(fmt.Sprintf("tool result[%d]", index), observed.Result, expected.ResultContains)...)
	failures = append(failures, containsNone(fmt.Sprintf("tool result[%d]", index), observed.Result, expected.ResultNotContains)...)
	return failures
}

// evaluateObservations 检查结构化 observation 是否按顺序出现。
func evaluateObservations(expect Expectation, result CaseResult) []string {
	var failures []string
	cursor := 0
	for index, expected := range expect.Observations {
		matchedAt := findExpectedObservation(result.Observations, expected, cursor)
		if matchedAt < 0 {
			failures = append(failures, fmt.Sprintf("missing expected observation[%d] type=%q name=%q", index, expected.Type, expected.Name))
			continue
		}
		cursor = matchedAt + 1
	}
	return failures
}

// evaluateApprovalRequests 检查结构化审批请求是否按顺序出现。
func evaluateApprovalRequests(expect Expectation, result CaseResult) []string {
	var failures []string
	if len(expect.ApprovalRequests) > len(result.ApprovalRequests) {
		failures = append(failures, fmt.Sprintf("approval requests = %d, want at least %d", len(result.ApprovalRequests), len(expect.ApprovalRequests)))
		return failures
	}
	for index, expected := range expect.ApprovalRequests {
		observed := result.ApprovalRequests[index]
		failures = append(failures, evaluateApprovalRequest(index, expected, observed)...)
	}
	return failures
}

// evaluateApprovalRequest 检查单个审批请求是否符合预期。
func evaluateApprovalRequest(index int, expected ExpectedApprovalRequest, observed ObservedApprovalRequest) []string {
	var failures []string
	if expected.ToolName != "" && observed.ToolName != expected.ToolName {
		failures = append(failures, fmt.Sprintf("approval request[%d] tool_name = %q, want %q", index, observed.ToolName, expected.ToolName))
	}
	if expected.RiskLevel != "" && observed.RiskLevel != expected.RiskLevel {
		failures = append(failures, fmt.Sprintf("approval request[%d] risk_level = %q, want %q", index, observed.RiskLevel, expected.RiskLevel))
	}
	failures = append(failures, containsAll(fmt.Sprintf("approval request[%d] reason", index), observed.Reason, expected.ReasonContains)...)
	return failures
}

// evaluateApprovalDecisions 检查结构化审批决策是否按顺序出现。
func evaluateApprovalDecisions(expect Expectation, result CaseResult) []string {
	var failures []string
	if len(expect.ApprovalDecisions) > len(result.ApprovalDecisions) {
		failures = append(failures, fmt.Sprintf("approval decisions = %d, want at least %d", len(result.ApprovalDecisions), len(expect.ApprovalDecisions)))
		return failures
	}
	for index, expected := range expect.ApprovalDecisions {
		observed := result.ApprovalDecisions[index]
		failures = append(failures, evaluateApprovalDecision(index, expected, observed)...)
	}
	return failures
}

// evaluateApprovalDecision 检查单个审批决策是否符合预期。
func evaluateApprovalDecision(index int, expected ExpectedApprovalDecision, observed ObservedApprovalDecision) []string {
	var failures []string
	if expected.ToolName != "" && observed.ToolName != expected.ToolName {
		failures = append(failures, fmt.Sprintf("approval decision[%d] tool_name = %q, want %q", index, observed.ToolName, expected.ToolName))
	}
	if expected.RiskLevel != "" && observed.RiskLevel != expected.RiskLevel {
		failures = append(failures, fmt.Sprintf("approval decision[%d] risk_level = %q, want %q", index, observed.RiskLevel, expected.RiskLevel))
	}
	if expected.Decision != "" && observed.Decision != expected.Decision {
		failures = append(failures, fmt.Sprintf("approval decision[%d] decision = %q, want %q", index, observed.Decision, expected.Decision))
	}
	if expected.Approver != "" && observed.Approver != expected.Approver {
		failures = append(failures, fmt.Sprintf("approval decision[%d] approver = %q, want %q", index, observed.Approver, expected.Approver))
	}
	failures = append(failures, containsAll(fmt.Sprintf("approval decision[%d] reason", index), observed.Reason, expected.ReasonContains)...)
	return failures
}

// findExpectedObservation 从指定位置开始查找符合预期的 observation。
func findExpectedObservation(observed []ObservedObservation, expected ExpectedObservation, start int) int {
	for index := start; index < len(observed); index++ {
		if expectedObservationMatches(expected, observed[index]) {
			return index
		}
	}
	return -1
}

// expectedObservationMatches 判断单个 observation 是否满足预期条件。
func expectedObservationMatches(expected ExpectedObservation, observed ObservedObservation) bool {
	if expected.Type != "" && observed.Type != expected.Type {
		return false
	}
	if expected.Name != "" && observed.Name != expected.Name {
		return false
	}
	if len(containsAll("observation content", observed.Content, expected.ContentContains)) > 0 {
		return false
	}
	if len(containsNone("observation content", observed.Content, expected.ContentNotContains)) > 0 {
		return false
	}
	return len(containsAll("observation error", observed.Error, expected.ErrorContains)) == 0
}

// evaluateToolErrors 检查工具错误是否符合预期，并默认拒绝未声明的工具错误。
func evaluateToolErrors(expect Expectation, result CaseResult) []string {
	if len(expect.ToolErrors) == 0 {
		if len(result.ToolErrors) > 0 {
			return []string{fmt.Sprintf("unexpected tool error: %s", result.ToolErrors[0].Error)}
		}
		return nil
	}
	var failures []string
	for _, expected := range expect.ToolErrors {
		if !matchesExpectedToolError(expected, result.ToolErrors) {
			failures = append(failures, fmt.Sprintf("missing expected tool error for %q", expected.Name))
		}
	}
	return failures
}

// containsAll 检查文本是否包含所有期望片段。
func containsAll(label string, text string, expected []string) []string {
	var failures []string
	for _, fragment := range expected {
		if !strings.Contains(text, fragment) {
			failures = append(failures, fmt.Sprintf("%s missing substring %q", label, fragment))
		}
	}
	return failures
}

// containsNone 检查文本是否不包含任何禁止片段。
func containsNone(label string, text string, forbidden []string) []string {
	var failures []string
	for _, fragment := range forbidden {
		if strings.Contains(text, fragment) {
			failures = append(failures, fmt.Sprintf("%s contains forbidden substring %q", label, fragment))
		}
	}
	return failures
}

// matchesExpectedToolError 判断实际工具错误集合是否满足单个期望。
func matchesExpectedToolError(expected ExpectedToolError, observed []ObservedToolError) bool {
	for _, toolError := range observed {
		if expected.Name != "" && toolError.Name != expected.Name {
			continue
		}
		if len(containsAll("tool error", toolError.Error, expected.ErrorContains)) == 0 {
			return true
		}
	}
	return false
}

// safeSessionID 根据用例名称和 mode 生成本地 memory session id。
func safeSessionID(caseName string, mode agent.Mode) string {
	replacer := strings.NewReplacer(" ", "_", "/", "_", "\\", "_", ":", "_")
	return replacer.Replace(caseName + "_" + string(mode))
}
