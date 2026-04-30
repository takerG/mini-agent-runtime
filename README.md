# mini-agent-runtime

A minimal Go CLI for streaming multi-turn chat from a local Ollama-compatible model API.

## Usage

Start an interactive multi-turn chat:

```powershell
go run . -model llama3.2
```

Then type one message per line. Use `/exit`, `/quit`, `exit`, or `quit` to leave.

You can also pass the first message as command arguments. After the first response, the CLI continues reading follow-up messages:

```powershell
go run . -model llama3.2 "介绍一下你自己"
```

Defaults:

- API URL: `http://localhost:11434/api/chat`
- Model: `llama3.2`

You can override defaults with flags:

```powershell
go run . -url http://localhost:11434/api/chat -model qwen2.5 "你好"
```

Or with environment variables:

```powershell
$env:LOCAL_MODEL_CHAT_URL = "http://localhost:11434/api/chat"
$env:LOCAL_MODEL_NAME = "qwen2.5"
go run . "你好"
```

## Streaming HTTP Proxy

Start a small HTTP server that streams model chunks to the client as soon as they arrive:

```powershell
go run . -serve 127.0.0.1:8080 -model qwen2.5
```

Call it with JSON:

```powershell
curl.exe -N -X POST http://127.0.0.1:8080/chat `
  -H "Content-Type: application/json" `
  -d "{\"message\":\"你好\"}"
```

The server posts to `http://localhost:11434/api/chat` with `stream: true`, reads each streamed JSON line, writes `message.content` to the HTTP response, and flushes every chunk immediately.

## Development

```powershell
$env:GOCACHE = "D:\work\mini-agent-runtime\.gocache"
go test ./...
go build -buildvcs=false ./...
```
