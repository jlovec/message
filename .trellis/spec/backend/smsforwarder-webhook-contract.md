# SmsForwarder Webhook Contract

> Executable contract for the Go SmsForwarder webhook service, PostgreSQL persistence, and 1Panel Docker deployment.

---

## Scenario: SmsForwarder Message Ingestion

### 1. Scope / Trigger

Use this contract when changing any of these boundaries:

- HTTP routes in `internal/webhook/server.go`
- Request payload fields accepted from SmsForwarder
- PostgreSQL storage in `internal/postgres/`
- Database migrations in `internal/postgres/migrations/`
- Docker or 1Panel deployment files: `Dockerfile`, `docker-compose.yml`, `.env.example`
- Mock sender behavior in `internal/smsforwarder/` or `cmd/smsforwarder-mock/`

This is an infra/cross-layer feature. A safe change must verify the full path:

```text
SmsForwarder JSON -> POST /webhook/smsforwarder -> webhook.Message -> postgres.Store.SaveMessage -> smsforwarder_messages table
```

---

### 2. Signatures

#### HTTP API

```text
GET  /healthz
POST /webhook/smsforwarder
```

No authentication is required by product requirement. Do not add auth checks unless the requirement changes.

#### Go Interfaces

````go
type Store interface {
    SaveMessage(context.Context, Message) (int64, error)
    ListMessages(context.Context, MessageQuery) ([]Message, error)
    MessageStats(context.Context, MessageQuery) (MessageStats, error)
    Ping(context.Context) error
}
```go
type Message struct {
    ID                 int64
    Source             string
    Sender             string
    SenderName         string
    Title              string
    Content            string
    OriginalContent    string
    Device             string
    ReceiveTime        string
    ForwarderTimestamp *int64
    CardSlot           string
    AppVersion         string
    RawPayload         json.RawMessage
    ConversationTitle  string
    CleanContent       string
    CreatedAt          string
    ProcessedAt        string
}
````

}

````

#### Mock Sender

```go
func Samples(device string) []smsforwarder.Payload
func Send(ctx context.Context, client *http.Client, endpoint string, payloads []smsforwarder.Payload) error
````

`Samples` must cover all supported message sources: `sms`, `wechat`, `feishu`, `qq`.

#### Deployment Commands

```bash
docker compose config
docker compose up -d --build
curl -sS http://127.0.0.1:18088/healthz
docker compose exec -T smsforwarder-webhook /app/smsforwarder-mock -url http://smsforwarder-webhook:8080/webhook/smsforwarder
```

---

#### Request Payload

`POST /webhook/smsforwarder` accepts JSON with this service-defined shape. New SmsForwarder phone-side templates must omit `source`; server-side `Message.Source` is inferred and persisted for source grouping.

```json
{
  "sender": "+8613800138000",
  "senderName": "中国移动",
  "title": "SIM1",
  "content": "+8613800138000\n验证码 123456\nSIM1\nSubId：0",
  "body": "验证码 123456",
  "originalContent": "验证码 123456",
  "device": "backup-phone",
  "receiveTime": "2026-06-10 10:00:00",
  "timestamp": "1781056800000",
  "cardSlot": "SIM1",
  "appVersion": "3.5.0.260224",
  "extra": {
    "mock": true
  }
}
```

Required fields:

| Field                  | Contract                                                                                                                     |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| Source inference input | At least one of `sender`, `from`, `title`, or `content` must allow inference to `sms`, `wechat`, `feishu`, or `qq`           |
| Message body input     | At least one of `body`, `rawContent`, `originalContent`, `message`, or `content` must be non-empty after `strings.TrimSpace` |

Optional fields are stored as empty strings when absent, except `timestamp`, which is stored as `NULL` when absent. Legacy `source` is accepted only as fallback when inference fails; inference wins over stale `source` values.

#### Body Normalization Contract

Persisted `Message.Content` must use the first non-empty value in this order:

```text
body -> rawContent -> originalContent -> message -> content
```

`Message.OriginalContent` must be set to the same clean body unless an explicit `originalContent` is present with `body/rawContent`.

#### Source Inference Contract

Infer source before consulting legacy `source` using this order:

| Input condition                                                                                           | Result           |
| --------------------------------------------------------------------------------------------------------- | ---------------- |
| `sender/from` contains `com.tencent.mm`                                                                   | `wechat`         |
| `sender/from` contains `lark` or `feishu`                                                                 | `feishu`         |
| `sender/from` contains `com.tencent.mobileqq` or `mobileqq`                                               | `qq`             |
| `sender/from` is non-empty and does not look like an Android package, or `title/content` has SMS metadata | `sms`            |
| none matched and legacy `source` is absent/unsupported                                                    | validation error |

#### Response Contract

| Route                        | Success        | Body                                 |
| ---------------------------- | -------------- | ------------------------------------ |
| `GET /healthz`               | `200 OK`       | `{"status":"ok"}`                    |
| `POST /webhook/smsforwarder` | `202 Accepted` | `{"status":"accepted","id":<db id>}` |

All responses must use `Content-Type: application/json`.

#### PostgreSQL Contract

Database and role used by this deployment:

```text
DATABASE_NAME=smsforwarder_messages
DATABASE_USER=smsforwarder_webhook
DATABASE_HOST=postgresql
DATABASE_PORT=5432
DATABASE_SSLMODE=disable
```

Table contract:

```sql
CREATE TABLE IF NOT EXISTS smsforwarder_messages (
    id BIGSERIAL PRIMARY KEY,
    source TEXT NOT NULL CHECK (source IN ('sms', 'wechat', 'feishu', 'qq')),
    sender TEXT NOT NULL DEFAULT '',
    sender_name TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL,
    original_content TEXT NOT NULL DEFAULT '',
    device TEXT NOT NULL DEFAULT '',
    receive_time TEXT NOT NULL DEFAULT '',
    forwarder_timestamp_millis BIGINT,
    card_slot TEXT NOT NULL DEFAULT '',
    app_version TEXT NOT NULL DEFAULT '',
    raw_payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`raw_payload` must contain the original request JSON bytes converted to JSON text for `JSONB` insertion.

#### Docker / 1Panel Contract

`docker-compose.yml` must attach the app to the external 1Panel network:

```yaml
networks:
  1panel-network:
    external: true
```

The service must publish the host endpoint on port `18088` by default:

```yaml
ports:
  - "${HOST_IP:-0.0.0.0}:${HOST_PORT:-18088}:${CONTAINER_PORT:-8080}"
```

For the current production topology, the app host is reachable from infra at `192.168.2.230`. The public `message.jlovec.net` 1Panel / OpenResty site must proxy directly to:

```text
http://192.168.2.230:18088
```

The OpenResty site proxy file on infra is:

```text
/root/www/sites/message.jlovec.net/proxy/root.conf
```

The `proxy_pass` for this site must not point to an SSH reverse tunnel endpoint such as `http://127.0.0.1:18089`, and the deployment must not require `ssh -R 127.0.0.1:18089:127.0.0.1:18088` for normal public access.

After changing the site proxy file, validate and reload OpenResty on infra:

```bash
docker exec openresty nginx -t
docker exec openresty nginx -s reload
```

The service must not define or start a second PostgreSQL container. It connects to the 1Panel PostgreSQL container named `postgresql` over `1panel-network`.

---

### 4. Validation & Error Matrix

| Case                                                                | Expected status / error                                  | Assertion point                     |
| ------------------------------------------------------------------- | -------------------------------------------------------- | ----------------------------------- |
| `GET /healthz` and DB ping succeeds                                 | `200 OK`, `{"status":"ok"}`                              | `handleHealthz`, `Store.Ping`       |
| `GET /healthz` and DB ping fails                                    | `503 Service Unavailable`, `database unavailable`        | `handleHealthz`                     |
| `POST /webhook/smsforwarder` valid no-source payload                | `202 Accepted`, returned `id`; inferred source persisted | `handleSmsForwarder`, `SaveMessage` |
| Source cannot be inferred and legacy `source` is absent/unsupported | `400 Bad Request`, source inference error                | `normalizePayload`                  |
| Empty/blank body aliases and `content`                              | `400 Bad Request`, `content is required`                 | `normalizeContent`                  |
| Invalid JSON                                                        | `400 Bad Request`                                        | `json.Unmarshal` boundary           |

| Body larger than 1 MiB or unreadable | `400 Bad Request` | `http.MaxBytesReader` |
| Wrong method on webhook | `405 Method Not Allowed`, `Allow: POST` | route handler |
| Wrong method on health check | `405 Method Not Allowed`, `Allow: GET` | route handler |
| PostgreSQL insert fails | `500 Internal Server Error` | `SaveMessage` error path |

---

### 5. Good / Base / Bad Cases

#### Good Case: Full No-Source SmsForwarder Payload

```json
{
  "sender": "com.tencent.mm",
  "senderName": "微信好友",
  "title": "微信好友",
  "content": "com.tencent.mm\n污染的通知聚合内容\n微信好友\nUID：0\n2026-06-10 10:00:00\nbackup-phone",
  "body": "微信通知内容",
  "originalContent": "微信通知内容",
  "device": "backup-phone",
  "receiveTime": "2026-06-10 10:00:00",
  "timestamp": "1781056800000",
  "cardSlot": "微信好友",
  "appVersion": "3.5.0.260224"
}
```

Expected: `202 Accepted`; database row has `source='wechat'`, `content='微信通知内容'`, and `raw_payload ? 'source' = false`.

#### Base Case: Minimal SMS Payload Without Source

```json
{
  "sender": "+8613800138000",
  "content": "验证码 123456"
}
```

Expected: `202 Accepted`; database row has `source='sms'`, optional text fields are persisted as empty strings, and `forwarder_timestamp_millis` is `NULL`.

#### Bad Case: Unsupported Legacy Source With No Inference Signal

```json
{
  "source": "email",
  "content": "hello"
}
```

Expected: `400 Bad Request` with error text `message source could not be inferred`.

#### Bad Case: Android Package Is Unknown

```json
{
  "sender": "com.example.unknown",
  "content": "hello"
}
```

Expected: `400 Bad Request` with error text `message source could not be inferred`.

#### Bad Case: Blank Content

```json
{
  "sender": "+8613800138000",
  "content": "   "
}
```

Expected: `400 Bad Request` with error text `content is required`.

---

### 6. Tests Required

Unit tests must cover:

- `internal/webhook/handler_test.go`
  - no-source payloads infer `sms`, `wechat`, `feishu`, and `qq`
  - clean body normalization prefers `body/rawContent/originalContent/message` over polluted `content`
  - stale legacy `source` is ignored when inference succeeds
  - unsupported legacy source or unknown Android package produces `400 Bad Request`
  - missing/blank body/content produces `400 Bad Request`
  - invalid JSON produces `400 Bad Request`
  - health check success returns JSON `{"status":"ok"}`
- `internal/smsforwarder/mock_test.go`
  - samples include exactly the supported source set as expected inference labels
  - marshaled mock JSON omits `source`
  - `Send` uses `POST`
  - `Send` sets `Content-Type: application/json`
  - sent bodies decode into JSON and include non-empty `body`

Before claiming completion, run:

```bash
docker run --rm \
  -e GOPROXY=https://goproxy.cn,direct \
  -e GOSUMDB=off \
  -v "$PWD":/src \
  -w /src \
  golang:1.23-bookworm \
  /bin/sh -lc 'export PATH=/usr/local/go/bin:/go/bin:$PATH; go mod tidy && gofmt -w $(find cmd internal -name "*.go" | sort) && go test ./... && go build ./cmd/server && go build ./cmd/smsforwarder-mock'
```

For deployment changes, also run:

```bash
docker compose config --quiet
docker compose up -d --build
curl -sS http://127.0.0.1:18088/healthz
docker compose exec -T smsforwarder-webhook /app/smsforwarder-mock -url http://smsforwarder-webhook:8080/webhook/smsforwarder
docker exec postgresql sh -lc 'psql -U "$POSTGRES_USER" -d smsforwarder_messages -tAc "select source, count(*) from smsforwarder_messages group by source order by source;"'
```

The final database output must include `sms`, `wechat`, `feishu`, and `qq`.

---

### 7. Wrong vs Correct

#### Wrong: Add a Separate Database Container

```yaml
services:
  postgres:
    image: postgres:16
```

Why wrong: this project must use the existing 1Panel PostgreSQL deployment, not create a separate database stack.

#### Correct: Join the 1Panel Network and Use `postgresql`

```yaml
services:
  smsforwarder-webhook:
    environment:
      DATABASE_HOST: ${DATABASE_HOST:-postgresql}
    networks:
      - 1panel-network

networks:
  1panel-network:
    external: true
```

#### Wrong: Drop Unknown SmsForwarder Fields

```go
msg.RawPayload = nil
```

Why wrong: SmsForwarder templates may evolve; preserving the full payload keeps audit and migration options open.

#### Correct: Preserve Raw JSON

```go
RawPayload: append(json.RawMessage(nil), rawPayload...)
```

#### Wrong: Persist `raw_payload` as Arbitrary Bytes

```go
[]byte(msg.RawPayload)
```

Why risky: the PostgreSQL column is `JSONB`; passing JSON text makes the DB boundary explicit.

#### Correct: Persist `raw_payload` as JSON Text

```go
string(msg.RawPayload)
```

---

## Scenario: Message Viewing UI, Message API, and Extracted Chat Fields

### 1. Scope / Trigger

Use this contract when changing any of these boundaries:

- Message-viewing routes in `internal/webhook/ui.go` and `internal/webhook/templates/messages.html`
- Extracted-field rules in `internal/webhook/enrichment.go`
- Message list and stats APIs exposed from `internal/webhook/server.go`
- PostgreSQL query/backfill logic in `internal/postgres/messages.go`
- Extraction schema migrations in `internal/postgres/migrations/002_add_message_enrichment.sql` and `internal/postgres/migrations/003_drop_message_suggestion_features.sql`

This is a cross-layer feature. A safe change must verify the full round-trip:

```text
SmsForwarder JSON -> normalizePayload -> EnrichMessage -> SaveMessage -> smsforwarder_messages extraction columns -> ListMessages/MessageStats -> /api/messages JSON -> /messages Pico CSS page
```

### 2. Signatures

#### HTTP API

```text
GET  /
GET  /healthz
GET  /messages
GET  /api/messages
GET  /api/messages/stats
POST /webhook/smsforwarder
```

`GET /` redirects to `/messages`.

#### Go Interfaces

```go
type Store interface {
    SaveMessage(context.Context, Message) (int64, error)
    ListMessages(context.Context, MessageQuery) ([]Message, error)
    MessageStats(context.Context, MessageQuery) (MessageStats, error)
    Ping(context.Context) error
}
```

```go
type MessageQuery struct {
    Source string `json:"source,omitempty"`
    Search string `json:"q,omitempty"`
    Limit  int    `json:"limit,omitempty"`
    Offset int    `json:"offset,omitempty"`
}
```

```go
type MessageStats struct {
    Total    int64
    BySource []MessageCount
}
```

#### Enrichment Entry Point

```go
func EnrichMessage(msg *Message)
func NormalizeMessageQuery(query MessageQuery) MessageQuery
```

`normalizePayload` and `postgres.Store.SaveMessage` both call `EnrichMessage`; the function must remain idempotent.

### 3. Contracts

#### Query Parameters

| Parameter | Contract                                                                                             |
| --------- | ---------------------------------------------------------------------------------------------------- |
| `source`  | Optional; only `sms`, `wechat`, `feishu`, `qq` survive normalization                                 |
| `q`       | Optional case-insensitive LIKE search over conversation title, clean content, title, and sender name |
| `limit`   | Defaults to `50`; maximum `200`                                                                      |
| `offset`  | Defaults to `0`; negative values clamp to `0`                                                        |

Unknown query parameters must be ignored and must not change list or stats behavior.

#### API Response Contract

`GET /api/messages` returns only stable first-class extraction fields:

```json
{
  "query": {
    "source": "qq",
    "q": "Claude",
    "limit": 50,
    "offset": 0
  },
  "messages": [
    {
      "id": 1,
      "source": "qq",
      "conversationTitle": "AI 交流群",
      "cleanContent": "Claude API 额度还有吗？",
      "processedAt": "2026-06-11T00:00:00.000Z"
    }
  ]
}
```

`GET /api/messages/stats` returns `total` and `bySource` using the same normalized filters as the list API.

#### PostgreSQL Extraction Columns

Migration `002_add_message_enrichment.sql` must add these stable columns:

```sql
conversation_title TEXT NOT NULL DEFAULT '',
clean_content TEXT NOT NULL DEFAULT '',
processed_at TIMESTAMPTZ
```

Required indexes:

```sql
idx_smsforwarder_messages_conversation_created_at
```

Migration `004_drop_unstable_message_fields.sql` must remove older unstable first-class extraction columns and their obsolete grouping index.
`NewStore` must run migrations before calling `BackfillMessageEnrichment`. Backfill currently processes rows where:

```sql
processed_at IS NULL OR clean_content = ''
```

This may include non-QQ sources; that is intentional so all messages can be displayed consistently, while QQ receives the special cleaning rules below.

#### Real Data Audit Contract

The current PostgreSQL audit found these real rows:

| Source   | Rows |
| -------- | ---- |
| `qq`     | 351  |
| `wechat` | 69   |
| `feishu` | 12   |

Stable raw fields across QQ, WeChat, and Feishu:

```text
title
sender
senderName
cardSlot
receiveTime
timestamp
device
content
originalContent
```

Stable extracted fields to persist and expose:

```text
conversationTitle
cleanContent
processedAt
```

Sparse content features such as links, `@` mentions, amount-like strings, number/code-like strings, and media markers should remain part of message content unless a future requirement explicitly asks for a separate extraction field.

#### QQ Cleaning Contract

For `source='qq'`:

| Input                                              | Enriched output                           |
| -------------------------------------------------- | ----------------------------------------- |
| `title='AI 交流群(3条新消息)'`                     | `conversation_title='AI 交流群'`          |
| `title='交流群(3条新消息)（2条新消息）'`           | `conversation_title='交流群'`             |
| content first line `小明：Claude API 额度还有吗？` | `clean_content='Claude API 额度还有吗？'` |
| URL-like first line `https://example.com:443/path` | must remain part of clean content         |

#### Frontend Contract

`GET /messages` must render the Pico CSS template from `internal/webhook/templates/messages.html`. The page must display backend-extracted fields and must not duplicate extraction logic in HTML.
`GET /messages` must render the Pico CSS template from `internal/webhook/templates/messages.html`. The page must display backend-extracted fields and must not duplicate extraction logic in HTML.

The “查看 JSON” link must be generated by Go using `url.Values` in `messagesAPIPath(query)`, then rendered as `{{.APIURL}}`. Do not hand-concatenate query strings in the template.

### 4. Validation & Error Matrix

| Case                                   | Expected status / error                    | Assertion point              |
| -------------------------------------- | ------------------------------------------ | ---------------------------- |
| `GET /messages` with valid filters     | `200 OK`, `text/html`                      | `handleMessagesPage`         |
| `GET /api/messages` with valid filters | `200 OK`, JSON with `query` and `messages` | `handleMessagesAPI`          |
| `GET /api/messages/stats`              | `200 OK`, grouped stats                    | `handleMessageStatsAPI`      |
| invalid method on page/API             | `405 Method Not Allowed` with `Allow`      | route handlers               |
| list/stats DB query fails              | `500 Internal Server Error`                | handler error path           |
| stale extra query parameters           | ignored                                    | `parseMessageQuery` boundary |

### 5. Good / Base / Bad Cases

#### Good Case: QQ Message With Extracted Fields

Input message:

```go
webhook.Message{
    Source:  "qq",
    Sender:  "com.tencent.mobileqq",
    Title:   "AI 交流群(3条新消息)",
    Content: "小明：Claude API 额度还有吗？",
}
```

Expected enrichment:

```text
conversationTitle=AI 交流群
cleanContent=Claude API 额度还有吗？
```

#### Base Case: Regular Chat Message

A QQ text message still receives `conversation_title`, `clean_content`, and `processed_at`; media/link markers remain in `clean_content`.

#### Bad Case: Manual URL Interpolation in Template

```html
<a href="/api/messages?source={{.Query.Source}}&q={{.Query.Search}}"
  >查看 JSON</a
>
```

Why wrong: this can corrupt spaces, Chinese text, and special characters. It also spreads query formatting into the template.

Correct:

```go
APIURL: messagesAPIPath(query)
```

```html
<a href="{{.APIURL}}" role="button" class="contrast">查看 JSON</a>
```

### 6. Tests Required

Unit/integration tests must cover:

- `internal/webhook/enrichment_test.go`
  - QQ unread suffix cleanup, including repeated suffixes
  - QQ author splitting with full-width and ASCII colon
  - URL-like content must not be split as author
  - media markers remain in clean content
  - query normalization clamps invalid source/limit/offset
- `internal/webhook/handler_test.go`
  - `/api/messages` filtering by source and search
  - `/api/messages/stats` grouped counts
  - stale extra query parameters do not affect output
  - `/messages` page renders Pico CSS, clean content, extracted fields, and no write-action controls
  - `/messages` page renders encoded `/api/messages` URL generated by Go
  - existing `/webhook/smsforwarder` behavior remains green

Before claiming completion, run:

```bash
docker run --rm   -e GOPROXY=https://goproxy.cn,direct   -e GOSUMDB=off   -v "$PWD":/src   -w /src   golang:1.23-bookworm   /bin/sh -lc 'git config --global --add safe.directory /src; export PATH=/usr/local/go/bin:/go/bin:$PATH; gofmt -w $(find cmd internal -name "*.go" | sort); go test ./...; go build -o /tmp/message-server ./cmd/server; go build -o /tmp/smsforwarder-mock ./cmd/smsforwarder-mock'
```

For runtime verification, also run:

```bash
docker build -t smsforwarder-webhook:local .
docker compose up -d --force-recreate
curl -sS http://127.0.0.1:18088/healthz
curl -sS 'http://127.0.0.1:18088/api/messages?source=qq&limit=5'
curl -sS 'http://127.0.0.1:18088/api/messages/stats'
curl -sS -i 'http://127.0.0.1:18088/messages' | sed -n '1,80p'
docker exec postgresql sh -lc 'psql -U "$POSTGRES_USER" -d smsforwarder_messages -c "select id, conversation_title, left(clean_content, 100) as clean_content_prefix, processed_at from smsforwarder_messages where source = '''qq''' order by id desc limit 20;"'
```

### 7. Wrong vs Correct

#### Wrong: Frontend-Only Extraction

```html
{{/* Do not strip author prefixes in the template. */}}
```

Why wrong: the extraction must be in the backend, and API/database consumers need the same fields.

#### Correct: Backend-Owned Extraction

```go
msg := Message{Source: "qq", Title: "AI 交流群(3条新消息)", Content: "小明：Claude API 额度还有吗？"}
EnrichMessage(&msg)
```

Then persist `conversation_title`, `clean_content`, and `processed_at`.

---

## Related Files

- `cmd/server/main.go`
- `cmd/smsforwarder-mock/main.go`
- `internal/webhook/server.go`
- `internal/webhook/ui.go`
- `internal/webhook/enrichment.go`
- `internal/webhook/handler_test.go`
- `internal/webhook/enrichment_test.go`
- `internal/webhook/templates/messages.html`
- `internal/postgres/store.go`
- `internal/postgres/messages.go`
- `internal/postgres/migrate.go`
- `internal/postgres/migrations/001_create_messages.sql`
- `internal/postgres/migrations/002_add_message_enrichment.sql`
- `internal/postgres/migrations/003_drop_message_suggestion_features.sql`
- `internal/smsforwarder/mock.go`
- `internal/smsforwarder/mock_test.go`
- `Dockerfile`
- `docker-compose.yml`
- `.env.example`
- `README.md`
- `docs/smsforwarder-config.md`
