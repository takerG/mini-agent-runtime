package errors

import (
	stderrors "errors"
	"strings"
	"testing"
)

// TestAppErrorKeepsCodeMessageAndNodeChain 验证 AppError 会保留错误码、消息和节点链路。
func TestAppErrorKeepsCodeMessageAndNodeChain(t *testing.T) {
	root := New(NodeToolRegistry, CodeToolNotFound, "unknown tool: missing_tool")
	wrapped := Wrap(NodeAgentToolCall, CodeToolExecutionFailed, root, "tool call failed")

	appErr := AsAppError(wrapped)
	if appErr == nil {
		t.Fatal("AsAppError returned nil, want AppError")
	}
	if got, want := appErr.Code, CodeToolExecutionFailed; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}
	if got, want := OriginNode(wrapped), NodeToolRegistry; got != want {
		t.Fatalf("origin node = %q, want %q", got, want)
	}
	if got, want := strings.Join(NodeChain(wrapped), " > "), "agent.tool_call > tools.registry"; got != want {
		t.Fatalf("node chain = %q, want %q", got, want)
	}
	if !stderrors.Is(wrapped, root) {
		t.Fatal("wrapped error does not unwrap to root error")
	}
}

// TestFormatForModelUsesStableToolErrorShape 验证交给模型的错误文本保持稳定结构。
func TestFormatForModelUsesStableToolErrorShape(t *testing.T) {
	err := Wrap(
		NodeAgentToolCall,
		CodeToolExecutionFailed,
		New(NodeCalculator, CodeToolInvalidArguments, "division by zero"),
		"calculator failed",
	)

	got := FormatForModel(err)
	for _, want := range []string{
		"tool error:",
		"code=tool_execution_failed",
		"origin=tools.calculator",
		"node_chain=agent.tool_call > tools.calculator",
		"message=calculator failed",
		"detail=division by zero",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("model error = %q, want substring %q", got, want)
		}
	}
}

// TestReporterPrintsDebugOnlyWhenEnabled 验证 reporter 只在 debug 开启时输出调试信息。
func TestReporterPrintsDebugOnlyWhenEnabled(t *testing.T) {
	err := Wrap(
		NodeAgentLoop,
		CodeModelRequestFailed,
		New(NodeOllamaClient, CodeUpstreamRequestFailed, "connection refused"),
		"model request failed",
	)

	var disabled strings.Builder
	NewReporter(false, &disabled).Debug(err)
	if got := disabled.String(); got != "" {
		t.Fatalf("disabled debug output = %q, want empty", got)
	}

	var enabled strings.Builder
	NewReporter(true, &enabled).Debug(err)
	got := enabled.String()
	for _, want := range []string{
		"[debug] error:",
		"code=model_request_failed",
		"origin=ollama.client",
		"node_chain=agent.loop > ollama.client",
		"message=model request failed",
		"detail=connection refused",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("debug output = %q, want substring %q", got, want)
		}
	}
}

// TestReporterLogAlwaysIncludesOriginAndNodeChain 验证普通错误日志始终包含发生节点和调用链。
func TestReporterLogAlwaysIncludesOriginAndNodeChain(t *testing.T) {
	err := Wrap(
		NodeServerProxy,
		CodeHTTPProxyFailed,
		New(NodeOllamaStream, CodeStreamDecodeFailed, "invalid json"),
		"stream proxy failed",
	)

	var output strings.Builder
	NewReporter(false, &output).Log(err)
	got := output.String()
	for _, want := range []string{
		"[error]",
		"code=http_proxy_failed",
		"origin=ollama.stream",
		"node_chain=server.proxy > ollama.stream",
		"message=stream proxy failed",
		"detail=invalid json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("log output = %q, want substring %q", got, want)
		}
	}
}
