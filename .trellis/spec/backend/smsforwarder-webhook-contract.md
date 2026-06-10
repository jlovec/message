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

```go
type Store interface {
    SaveMessage(context.Context, Message) (int64, error)
    Ping(context.Context) error
}
```

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
    Sign               string
    AppPackage         string
    CardSlot           string
    AppVersion         string
    RawPayload         json.RawMessage
}
```

#### Mock Sender

```go
func Samples(device string) []smsforwarder.Payload
func Send(ctx context.Context, client *http.Client, endpoint string, payloads []smsforwarder.Payload) error
```

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

`POST /webhook/smsforwarder` accepts JSON with this service-defined shape. New SmsForwarder phone-side templates must omit `source`; server-side `Message.Source` is inferred and persisted for database categorization.

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
  "sign": "",
  "cardSlot": "SIM1",
  "appVersion": "3.5.0.260224",
  "extra": {
    "mock": true
  }
}
```

Required fields:

| Field                  | Contract                                                                                                                                        |
| ---------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| Source inference input | At least one of `sender`, `from`, `appPackage`, `packageName`, `title`, or `content` must allow inference to `sms`, `wechat`, `feishu`, or `qq` |
| Message body input     | At least one of `body`, `rawContent`, `originalContent`, `message`, or `content` must be non-empty after `strings.TrimSpace`                    |

Optional fields are stored as empty strings when absent, except `timestamp`, which is stored as `NULL` when absent. Legacy `source` is accepted only as fallback when inference fails; inference wins over stale `source` values.

#### Body Normalization Contract

Persisted `Message.Content` must use the first non-empty value in this order:

```text
body -> rawContent -> originalContent -> message -> content
```

`Message.OriginalContent` must be set to the same clean body unless an explicit `originalContent` is present with `body/rawContent`.

#### Source Inference Contract

Infer source before consulting legacy `source` using this order:

| Input condition                                                                                                             | Result           |
| --------------------------------------------------------------------------------------------------------------------------- | ---------------- |
| `sender`, `from`, `appPackage`, or `packageName` contains `com.tencent.mm`                                                  | `wechat`         |
| package hints contain `lark` or `feishu`                                                                                    | `feishu`         |
| package hints contain `com.tencent.mobileqq` or `mobileqq`                                                                  | `qq`             |
| `sender/from` is non-empty and does not look like an Android package, or `title/content` contains `SIM`, `SubId`, or `卡槽` | `sms`            |
| none matched and legacy `source` is absent/unsupported                                                                      | validation error |

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
    sign TEXT NOT NULL DEFAULT '',
    app_package TEXT NOT NULL DEFAULT '',
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
  "sign": "",
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

## Related Files

- `cmd/server/main.go`
- `cmd/smsforwarder-mock/main.go`
- `internal/webhook/server.go`
- `internal/webhook/handler_test.go`
- `internal/postgres/store.go`
- `internal/postgres/migrate.go`
- `internal/postgres/migrations/001_create_messages.sql`
- `internal/smsforwarder/mock.go`
- `internal/smsforwarder/mock_test.go`
- `Dockerfile`
- `docker-compose.yml`
- `.env.example`
- `docs/smsforwarder-config.md`
