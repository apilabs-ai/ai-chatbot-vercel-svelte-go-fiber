# aichat_go

Go API for the AI chat widget (Fiber v3, OpenAI streaming).

## Run

```bash
cp .env.example .env   # if present; set OPENAI_API_KEY
go run ./cmd/api
```

## Tests

```bash
go test ./...
```

With coverage:

```bash
go test ./... -cover
```

Packages under test:

| Package | Focus |
|--------|--------|
| `internal/chat` | `StreamParser`, `Service` (mock provider) |
| `internal/store` | In-memory conversations and messages |
| `internal/config` | `Load()` and env defaults |
| `internal/api` | HTTP handlers, SSE stream, tenant middleware, error handler |
| `internal/providers/openai` | Intent routing, structured parts, deterministic stream |
| `internal/observability` | Request logger + request ID |

The OpenAI provider’s live HTTP calls are **not** exercised in unit tests; deterministic modes (artifact/plan/etc.) are covered without the network.

## Configuration

| Env | Default | Purpose |
|-----|---------|---------|
| `OPENAI_API_KEY` | — (required) | OpenAI key |
| `OPENAI_MODEL` | `gpt-4o-mini` | Default model |
| `OPENAI_MODELS` | `$OPENAI_MODEL,gpt-4o,gpt-5-mini,o4-mini` | Allow-list for the model selector; first entry is the default. Requests for any other model get `400` |

## API

| Method | Path | Notes |
|--------|------|-------|
| `POST` | `/v1/responses:stream` | Body: `input`, `conversation_id?`, `model?`, `tools?` (`["web_search","image_generation"]`). Returns SSE |
| `GET` | `/v1/models` | `[{id, reasoning, default}]` |
| `GET` | `/v1/conversations` | |
| `GET` | `/v1/conversations/:id/messages` | |
| `POST` | `/v1/conversations/:id/restore` | Body: `message_id`. Drops every message after it (checkpoint) |

SSE events: `message.delta`, `reasoning.delta` (reasoning models), `part`, `usage`, `message.completed` (`conversation_id`, `user_message_id`, `message_id`), `error`.

Part types: `code`, `plan`, `queue`, `artifact`, `confirmation`, `citation`, `sources` (web search citations), `image` (base64 in `content`, `meta.media_type`).

**Note:** messages are stored in memory, text only. Reasoning and generated images are not persisted, so they don't come back when a conversation is reloaded from `/messages`.
