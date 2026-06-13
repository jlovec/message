# SmsForwarder Webhook

一个使用 **Go** 实现、按 **Trellis** 初始化管理的简单 webhook 服务，用于接收手机端 **SmsForwarder** 转发过来的短信与 App 通知，并写入 **1Panel PostgreSQL**。

## 已实现能力

- 无认证 webhook：`POST /webhook/smsforwarder`
- 健康检查：`GET /healthz`
- 简洁消息查看页面：`GET /messages`
- 消息列表 API：`GET /api/messages`
- 消息统计 API：`GET /api/messages/stats`
- 支持消息来源：`sms`、`wechat`、`feishu`、`qq`
- 后端字段抽取：会话标题、清洗正文、处理时间
- 基于当前真实 QQ / 微信 / 飞书数据审计稳定字段，页面只展示可复用的抽取结果
- 原始 JSON payload 全量落库，方便后续审计和扩展
- 自动执行 PostgreSQL 初始化迁移，并在服务启动时回填未处理历史消息的抽取字段
- Docker / Docker Compose 部署，接入 `1panel-network`
- 内置 mock 发送器：`cmd/smsforwarder-mock`

## 目录结构

```text
cmd/
├── server/                 # Webhook 服务入口
└── smsforwarder-mock/      # Mock 发送器入口
internal/
├── config/                 # 环境变量配置加载
├── postgres/               # PG 存储、迁移、消息列表/统计/回填查询
├── smsforwarder/           # Mock payload 生成与发送
└── webhook/                # HTTP 路由、请求校验、字段抽取、Pico CSS 页面模板
```

## 消息 JSON 格式

手机端推荐使用**无 `source` 字段**的统一 JSON 模板；服务端会基于 `sender` / `from` / 标题与短信元信息自动识别 `sms`、`wechat`、`feishu`、`qq` 并写入数据库。

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

### 字段说明

| 字段                    | 必填     | 说明                                                                                      |
| ----------------------- | -------- | ----------------------------------------------------------------------------------------- |
| `source`                | 否       | 仅兼容旧模板；新配置应省略。服务端优先自动推断并持久化 `sms` / `wechat` / `feishu` / `qq` |
| `sender` / `from`       | 推荐     | 发送方号码或 App 包名；推荐填 SmsForwarder 的 `[from]`，这是类型识别的主要依据            |
| `senderName` / `name`   | 否       | 发送方显示名                                                                              |
| `title`                 | 否       | 短信卡槽或通知标题；可辅助识别短信的 SIM/SubId 元信息                                     |
| `body` / `rawContent`   | 推荐     | 干净正文；服务端会优先作为入库 `content`                                                  |
| `originalContent`       | 推荐     | 原始干净正文；当 `body` 为空时优先作为入库 `content`                                      |
| `message`               | 否       | 正文别名；读取顺序低于 `body/rawContent/originalContent`                                  |
| `content`               | 条件必填 | SmsForwarder 转发内容；当没有 `body/rawContent/originalContent/message` 时作为兜底正文    |
| `device` / `deviceMark` | 否       | 手机端 `device_mark`                                                                      |
| `receiveTime` / `time`  | 否       | 手机端接收时间                                                                            |
| `timestamp`             | 否       | 毫秒时间戳字符串                                                                          |
| `cardSlot`              | 否       | SIM 卡槽 / 通知标题补充字段                                                               |
| `appVersion`            | 否       | SmsForwarder 应用版本                                                                     |
| `extra`                 | 否       | 扩展字段，服务端原样保留在 `raw_payload` 中                                               |

正文入库读取顺序：`body` → `rawContent` → `originalContent` → `message` → `content`。

## 简洁消息页面

本项目为消息查看选择了 **Pico CSS + Go `html/template` 服务端渲染**：

- 官方资料：https://picocss.com/，Pico CSS 是 “Minimal CSS Framework for semantic HTML”。
- 选型原因：语义化 HTML、默认简洁美观、无需 Node.js 构建链，适合当前 Go 单服务。
- 页面地址：`http://127.0.0.1:18088/messages`，公网部署后为 `https://message.jlovec.net/messages`。
- 页面能力：来源筛选、关键词搜索、统计卡片、消息列表，以及后端抽取字段展示。
- 页面展示字段：会话标题、清洗正文、原始标题、发送方名称、设备、接收时间。

## 消息 API

| 方法与路径                | 用途         | 主要参数 / 返回值                          |
| ------------------------- | ------------ | ------------------------------------------ |
| `GET /api/messages`       | 查询消息列表 | 查询参数：`source`、`q`、`limit`、`offset` |
| `GET /api/messages/stats` | 查询统计     | 返回 `total`、`bySource`                   |

`source` 只接受 `sms`、`wechat`、`feishu`、`qq`；`q` 会搜索会话标题、清洗正文、原始标题和发送方名称。

## 当前数据可抽取字段

基于当前 PostgreSQL 中的真实聊天数据审计：QQ 351 条、微信 69 条、飞书 12 条。三类 App 通知稳定具备以下原始字段：

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

服务端会进一步持久化这些抽取字段：

| 数据库字段           | API 字段            | 含义                                                     |
| -------------------- | ------------------- | -------------------------------------------------------- |
| `conversation_title` | `conversationTitle` | 会话标题；QQ 会去除 `(N条新消息)` / `（N条新消息）` 后缀 |
| `clean_content`      | `cleanContent`      | 清洗正文；QQ 会去掉正文首行中稳定可识别的作者前缀        |
| `processed_at`       | `processedAt`       | 后端加工/回填时间                                        |

真实数据里还零散出现链接、`@`、金额样式、数字码样式等内容特征；它们保留在正文中，不单独做成页面功能。

## 数据库

本项目会把消息写入 1Panel PostgreSQL 容器 `postgresql` 中的新数据库：

- Database: `smsforwarder_messages`
- User: `smsforwarder_webhook`
- Password: 已写入本地 `.env`

表结构见：

- `internal/postgres/migrations/001_create_messages.sql`：基础消息表。
- `internal/postgres/migrations/002_add_message_enrichment.sql`：字段抽取扩展列与索引。
- `internal/postgres/migrations/003_drop_message_suggestion_features.sql`：移除上一版不再使用的扩展列与索引。
- `internal/postgres/migrations/004_drop_unstable_message_fields.sql`：移除本轮判定不稳定的一等字段与索引。

## 部署

### 1. 环境变量

实际可运行配置已经写入本地 `.env`。
如需在别处部署，可参考 `.env.example`。

### 2. 启动本机服务

```bash
docker compose up -d --build
```

服务默认发布到 `0.0.0.0:18088`，当前生产机也可通过内网地址 `http://192.168.2.230:18088` 访问。

### 3. 1Panel / OpenResty 公网反向代理

两台服务器网络互通时，`message.jlovec.net` 的 1Panel / OpenResty 反向代理 upstream 必须直接指向：

```text
http://192.168.2.230:18088
```

不要把公网链路建立在 `ssh -R` 反向隧道或 infra 本机 `127.0.0.1:18089` 上。当前站点配置文件为 infra 机器上的：

```text
/root/www/sites/message.jlovec.net/proxy/root.conf
```

核心配置：

```nginx
location ^~ / {
    proxy_pass http://192.168.2.230:18088;
}
```

修改后需要在 infra 上执行 `docker exec openresty nginx -t` 并 reload OpenResty。

### 4. 检查健康状态

```bash
curl http://127.0.0.1:18088/healthz
curl http://192.168.2.230:18088/healthz
ssh infra 'curl -fsS http://192.168.2.230:18088/healthz'
curl https://message.jlovec.net/healthz
curl 'https://message.jlovec.net/messages?limit=1'
curl 'https://message.jlovec.net/api/messages?source=qq&limit=5'
curl 'https://message.jlovec.net/api/messages/stats'
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

查看最新消息的抽取字段：

```bash
docker exec postgresql sh -lc '
psql -U "$POSTGRES_USER" -d smsforwarder_messages -c "select id, source, conversation_title, left(clean_content, 80) as clean_content_prefix, processed_at from smsforwarder_messages order by id desc limit 20;"'
```

只看 QQ 清洗回填效果：

```bash
docker exec postgresql sh -lc '
psql -U "$POSTGRES_USER" -d smsforwarder_messages -c "select id, conversation_title, left(clean_content, 100) as clean_content_prefix, processed_at from smsforwarder_messages where source = '\''qq'\'' order by id desc limit 20;"'
```

## SmsForwarder 配置文档

详见：`docs/smsforwarder-config.md`
