package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	apperrors "mini-agent-runtime/internal/errors"
	"mini-agent-runtime/internal/ollama"
)

type proxyChatRequest struct {
	// 外部调用 /chat 时只需要传 message。
	// 示例：
	//   {"message":"你好"}
	Message string `json:"message"`

	// Model 是可选字段。调用方可以针对单次请求覆盖默认模型：
	//   {"model":"qwen2.5","message":"你好"}
	Model string `json:"model,omitempty"`
}

func NewChatProxyHandler(endpoint string, defaultModel string, client *http.Client) http.Handler {
	// 把 http.Client 作为参数传进来，方便测试时注入假的客户端。
	// 真实运行时传 nil 或 http.DefaultClient 即可。
	if client == nil {
		client = http.DefaultClient
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 第一版只开放 POST /chat。
		// 用 POST 是因为用户消息放在请求体里，比 GET 查询参数更适合长文本。
		if r.Method != http.MethodPost {
			http.Error(w, apperrors.New(apperrors.NodeServerProxy, apperrors.CodeInvalidUserInput, "method not allowed").Error(), http.StatusMethodNotAllowed)
			return
		}

		// 解析外部调用方传进来的 JSON。
		// 这个 JSON 是我们自己定义的简化协议，不要求调用方了解 Ollama 的 messages 结构。
		var inbound proxyChatRequest
		if err := json.NewDecoder(r.Body).Decode(&inbound); err != nil {
			http.Error(w, apperrors.Wrap(apperrors.NodeServerProxy, apperrors.CodeInvalidUserInput, err, "invalid request body").Error(), http.StatusBadRequest)
			return
		}

		// 没有 message 就没有可发送给模型的用户输入，因此直接返回 400。
		if inbound.Message == "" {
			http.Error(w, apperrors.New(apperrors.NodeServerProxy, apperrors.CodeInvalidUserInput, "message is required").Error(), http.StatusBadRequest)
			return
		}

		// 如果请求体里没有指定 model，就使用启动服务时的默认模型。
		model := inbound.Model
		if model == "" {
			model = defaultModel
		}

		// 使用 r.Context() 很重要：
		// 如果浏览器、curl 或前端页面中途断开，Go 会取消这个 context，
		// 上游的模型请求也会跟着取消。agent 系统里，这能减少无意义的后台生成。
		upstreamReq, err := ollama.NewChatRequestWithContext(r.Context(), endpoint, model, inbound.Message)
		if err != nil {
			http.Error(w, apperrors.Wrap(apperrors.NodeServerProxy, apperrors.CodeRequestBuildFailed, err, "build upstream request").Error(), http.StatusInternalServerError)
			return
		}

		// 发起到本地模型服务的上游请求。
		// 注意：这里拿到响应后不会 io.ReadAll，因为 io.ReadAll 会等完整响应结束；
		// 我们要把 upstreamResp.Body 保持为流，交给 ollama.StreamChatContent 边读边转发。
		upstreamResp, err := client.Do(upstreamReq)
		if err != nil {
			http.Error(w, apperrors.Wrap(apperrors.NodeServerProxy, apperrors.CodeUpstreamRequestFailed, err, "post chat request").Error(), http.StatusBadGateway)
			return
		}
		defer upstreamResp.Body.Close()

		if upstreamResp.StatusCode < 200 || upstreamResp.StatusCode > 299 {
			http.Error(w, apperrors.New(apperrors.NodeServerProxy, apperrors.CodeUpstreamStatusFailed, fmt.Sprintf("chat request failed: %s", upstreamResp.Status)).Error(), http.StatusBadGateway)
			return
		}

		// 对外返回纯文本流。
		// 浏览器或 curl 会持续收到模型的文本片段，而不是一整个 JSON 对象。
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		// 禁用缓存，避免中间层或浏览器把流式响应攒起来再显示。
		w.Header().Set("Cache-Control", "no-cache")

		// 避免浏览器猜测响应类型。虽然不是流式的核心，但这是 HTTP 服务的好习惯。
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// 这是 HTTP 代理模式的核心：
		//   上游：本地模型按行返回 JSON
		//   中间：ollama.StreamChatContent 解析每行 JSON，取出 message.content
		//   下游：把 content 立即写给 HTTP 调用方，并在每个 chunk 后 Flush
		if err := ollama.StreamChatContent(upstreamResp.Body, w); err != nil {
			http.Error(w, apperrors.Wrap(apperrors.NodeServerProxy, apperrors.CodeHTTPProxyFailed, err, "stream proxy failed").Error(), http.StatusBadGateway)
			return
		}
	})
}
