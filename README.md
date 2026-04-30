# mini-agent-runtime

A minimal Go CLI for streaming one chat message from a local Ollama-compatible model API.

## Usage

Run with a message as command arguments:

```powershell
go run . -model llama3.2 "介绍一下你自己"
```

Or type one line after launch:

```powershell
go run . -model llama3.2
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
