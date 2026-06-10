# SmsForwarder Webhook

一个使用 **Go** 实现、按 **Trellis** 初始化管理的简单 webhook 服务，用于接收手机端 **SmsForwarder** 转发过来的消息，并写入 **1Panel PostgreSQL**。

## 已实现能力

- 无认证 webhook：`POST /webhook/smsforwarder`
- 健康检查：`GET /healthz`
- 支持消息来源：`sms`、`wechat`、`feishu`、`qq`
- 原始 JSON payload 全量落库，方便后续审计和扩展
- 自动执行 PostgreSQL 初始化迁移
- Docker / Docker Compose 部署，接入 `1panel-network`
- 内置 mock 发送器：`cmd/smsforwarder-mock`

## 目录结构

```text
cmd/
├── server/                 # Webhook 服务入口
└── smsforwarder-mock/      # Mock 发送器入口
internal/
├── config/                 # 环境变量配置加载
├── postgres/               # PG 存储与迁移
├── smsforwarder/           # Mock payload 生成与发送
└── webhook/                # HTTP 路由与请求校验
```

## 消息 JSON 格式

手机端推荐使用**无 `source` 字段**的统一 JSON 模板；服务端会基于 `sender` / `from` / `appPackage` / 标题与短信元信息自动识别 `sms`、`wechat`、`feishu`、`qq` 并写入数据库。

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
  "appPackage": "",
  "cardSlot": "SIM1",
  "appVersion": "3.5.0.260224",
  "extra": {
    "mock": true
  }
}
```

### 字段说明

| 字段                         | 必填     | 说明                                                                                      |
| ---------------------------- | -------- | ----------------------------------------------------------------------------------------- |
| `source`                     | 否       | 仅兼容旧模板；新配置应省略。服务端优先自动推断并持久化 `sms` / `wechat` / `feishu` / `qq` |
| `sender` / `from`            | 推荐     | 发送方号码或 App 包名；推荐填 SmsForwarder 的 `[from]`，这是类型识别的主要依据            |
| `senderName` / `name`        | 否       | 发送方显示名                                                                              |
| `title`                      | 否       | 短信卡槽或通知标题；可辅助识别短信的 SIM/SubId 元信息                                     |
| `body` / `rawContent`        | 推荐     | 干净正文；服务端会优先作为入库 `content`                                                  |
| `originalContent`            | 推荐     | 原始干净正文；当 `body` 为空时优先作为入库 `content`                                      |
| `message`                    | 否       | 正文别名；优先级低于 `body/rawContent/originalContent`                                    |
| `content`                    | 条件必填 | SmsForwarder 转发内容；当没有 `body/rawContent/originalContent/message` 时作为兜底正文    |
| `device` / `deviceMark`      | 否       | 手机端 `device_mark`                                                                      |
| `receiveTime` / `time`       | 否       | 手机端接收时间                                                                            |
| `timestamp`                  | 否       | 毫秒时间戳字符串                                                                          |
| `sign`                       | 否       | SmsForwarder 可选签名，本服务仅存储不校验                                                 |
| `appPackage` / `packageName` | 否       | App 包名；通知类消息可辅助识别类型                                                        |
| `cardSlot`                   | 否       | SIM 卡槽 / 通知标题补充字段                                                               |
| `appVersion`                 | 否       | SmsForwarder 应用版本                                                                     |
| `extra`                      | 否       | 扩展字段，服务端原样保留在 `raw_payload` 中                                               |

正文入库优先级：`body` → `rawContent` → `originalContent` → `message` → `content`。

## 数据库

本项目会把消息写入 1Panel PostgreSQL 容器 `postgresql` 中的新数据库：

- Database: `smsforwarder_messages`
- User: `smsforwarder_webhook`
- Password: 已写入本地 `.env`

表结构见：`internal/postgres/migrations/001_create_messages.sql`

## 部署

### 1. 环境变量

实际可运行配置已经写入本地 `.env`。
如需在别处部署，可参考 `.env.example`。

### 2. 启动

```bash
docker compose up -d --build
```

### 3. 检查健康状态

```bash
curl http://127.0.0.1:18088/healthz
```

## Mock 测试

### 方式一：本地直接运行 mock

```bash
docker run --rm \
  -v "$PWD":/src \
  -w /src \
  -e GOPROXY=https://goproxy.cn,direct \
  -e GOSUMDB=off \
  golang:1.23-bookworm \
  go run ./cmd/smsforwarder-mock -url http://127.0.0.1:18088/webhook/smsforwarder
```

### 方式二：构建后在容器内执行 mock 二进制

```bash
docker compose exec smsforwarder-webhook /app/smsforwarder-mock \
  -url http://smsforwarder-webhook:8080/webhook/smsforwarder
```

## 查询数据

```bash
docker exec postgresql sh -lc '
psql -U "$POSTGRES_USER" -d smsforwarder_messages -c "select id, source, sender, created_at from smsforwarder_messages order by id desc limit 20;"'
```

## SmsForwarder 配置文档

详见：`docs/smsforwarder-config.md`
