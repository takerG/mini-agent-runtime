package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip 让测试可以用函数模拟 http.RoundTripper。
func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

// TestChatProxyHandlerStreamsModelContentToClient 验证 HTTP 代理会把模型流式内容转发给客户端。
func TestChatProxyHandlerStreamsModelContentToClient(t *testing.T) {
	var upstreamBody string
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("read upstream request body: %v", err)
			}
			upstreamBody = string(body)

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"message":{"content":"hello"}}` + "\n" +
						`{"message":{"content":" world"}}` + "\n" +
						`{"done":true}` + "\n",
				)),
			}, nil
		}),
	}
	handler := NewChatProxyHandler("http://localhost:11434/api/chat", "llama3.2", client)
	req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"message":"say hi"}`))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got, want := recorder.Body.String(), "hello world"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
	if !recorder.Flushed {
		t.Fatal("response was not flushed")
	}
	if !strings.Contains(upstreamBody, `"stream":true`) {
		t.Fatalf("upstream body = %q, want stream true", upstreamBody)
	}
	if !strings.Contains(upstreamBody, `"content":"say hi"`) {
		t.Fatalf("upstream body = %q, want user message", upstreamBody)
	}
}
