# SmsForwarder 配置文档

本文档用于把 Android 手机上的 [SmsForwarder](https://github.com/pppscn/SmsForwarder) 配置为向本项目 webhook 转发短信与 App 通知（微信、飞书、QQ）。

> 本 webhook **无需认证**。如果你后续暴露到公网，建议放在反向代理后面，用 IP 白名单、Basic Auth 或随机路径做额外保护。

## 1. 服务端信息

当前公网 webhook 地址为：

```text
https://message.jlovec.net/webhook/smsforwarder
```

公网健康检查地址：

```text
https://message.jlovec.net/healthz
```

本机测试地址：

```text
http://127.0.0.1:18088/webhook/smsforwarder
```

生产机内网地址：

```text
http://192.168.2.230:18088/webhook/smsforwarder
```

1Panel / OpenResty 反向代理说明：两台服务器网络互通时，infra 上的 `message.jlovec.net` upstream 应直接使用 `http://192.168.2.230:18088`。不要把公网链路建立在 `ssh -R` 反向隧道或 infra 本机 `127.0.0.1:18089` 上。

请求方式：

- Method：`POST`
- Content-Type：`application/json;charset=utf-8`
- 鉴权：无

## 2. 服务端接收 JSON 格式

手机端推荐使用**统一无 `source` 字段**的 JSON 模板。`source` 仍会作为服务端内部来源写入数据库，但不需要再在 SmsForwarder 的每条规则里手动配置。

```json
{
  "sender": "[from]",
  "senderName": "[title]",
  "title": "[title]",
  "content": "[content]",
  "body": "[org_content]",
  "originalContent": "[org_content]",
  "device": "[device_mark]",
  "receiveTime": "[receive_time]",
  "timestamp": "[timestamp]",
  "cardSlot": "[card_slot]",
  "appVersion": "[app_version]"
}
```

字段说明：

| 字段                    | 必填     | 推荐来源         | 说明                                                                                      |
| ----------------------- | -------- | ---------------- | ----------------------------------------------------------------------------------------- |
| `source`                | 否       | 不填             | 仅兼容旧模板；新配置应省略。服务端会自动推断并持久化 `sms` / `wechat` / `feishu` / `qq`   |
| `sender` / `from`       | 推荐     | `[from]`         | 短信号码，或 App 通知包名；这是类型识别的主要依据                                         |
| `senderName` / `name`   | 否       | `[title]`        | 通知标题 / 发送方名称                                                                     |
| `title`                 | 否       | `[title]`        | 短信卡槽或 App 通知标题；可辅助识别短信 SIM/SubId 元信息                                  |
| `body` / `rawContent`   | 推荐     | `[org_content]`  | 干净正文；服务端优先写入数据库 `content`                                                  |
| `originalContent`       | 推荐     | `[org_content]`  | 原始干净正文；当 `body` 为空时作为入库正文                                                |
| `message`               | 否       | `[msg]`          | 正文别名；读取顺序低于 `body/rawContent/originalContent`                                  |
| `content`               | 条件必填 | `[content]`      | 转发内容；真实测试中可能包含号码/标题/UID/时间/设备等元信息，仅在没有更干净字段时兜底入库 |
| `device` / `deviceMark` | 否       | `[device_mark]`  | SmsForwarder 通用设置里的设备名称                                                         |
| `receiveTime` / `time`  | 否       | `[receive_time]` | 手机端接收时间                                                                            |
| `timestamp`             | 否       | `[timestamp]`    | SmsForwarder 发送时的毫秒时间戳                                                           |
| `cardSlot`              | 否       | `[card_slot]`    | 短信/来电 SIM 卡槽，或 App 通知标题                                                       |
| `appVersion`            | 否       | `[app_version]`  | SmsForwarder App 版本                                                                     |

正文入库读取顺序：`body` → `rawContent` → `originalContent` → `message` → `content`。

类型识别规则：

| 输入特征                                                                      | 服务端识别为      |
| ----------------------------------------------------------------------------- | ----------------- |
| `sender/from` 包含 `com.tencent.mm`                                           | `wechat`          |
| 包名包含 `lark` 或 `feishu`，例如 `com.ss.android.lark`                       | `feishu`          |
| 包名包含 `com.tencent.mobileqq` 或 `mobileqq`                                 | `qq`              |
| `sender/from` 是号码、非 Android 包名，或标题/正文包含 `SIM`、`SubId`、`卡槽` | `sms`             |
| 无法识别                                                                      | `400 Bad Request` |

## 3. SmsForwarder Webhook 通道配置

在 SmsForwarder 中新增一个 Webhook 发送通道，推荐配置：

```text
名称：message-webhook
请求方式：POST
WebServer：https://message.jlovec.net/webhook/smsforwarder
Secret：留空
WebParams：填写第 2 节的统一 JSON 模板
```

SmsForwarder 官方 wiki 说明：当 `webParams` 非空且以 `{` 开头时，会按 JSON 发送，并使用：

```text
Content-Type: application/json;charset=utf-8
```

## 4. 转发规则配置

所有规则都使用同一个 `message-webhook` 通道和同一份无 `source` JSON 模板；区别只在 SmsForwarder 规则本身如何匹配短信或 App。

### 4.1 短信规则

```text
类型：短信
匹配：全部短信（或按你的需求设置号码/关键词过滤）
发送通道：message-webhook
```

短信会通过号码型 `[from]` 或 `SIM/SubId/卡槽` 元信息识别为 `sms`。

### 4.2 微信通知规则

```text
类型：App 通知
App 包名：com.tencent.mm
发送通道：message-webhook
```

服务端会根据包名 `com.tencent.mm` 识别为 `wechat`。

### 4.3 飞书通知规则

飞书国际版/国内版包名可能因版本不同略有差异，常见包名：

```text
com.ss.android.lark
```

如果你的手机上包名不同，以 SmsForwarder 的 App 选择页面显示为准。服务端会根据包名里的 `lark` 或 `feishu` 识别为 `feishu`。

### 4.4 QQ 通知规则

```text
类型：App 通知
App 包名：com.tencent.mobileqq
发送通道：message-webhook
```

服务端会根据包名 `com.tencent.mobileqq` 或 `mobileqq` 识别为 `qq`。

## 5. 可用变量说明

本项目主要使用 SmsForwarder webhook 模板变量：

| SmsForwarder 变量 | 含义                          |
| ----------------- | ----------------------------- |
| `[from]`          | 短信来源号码 / App 包名       |
| `[content]`       | 经过模板处理后的消息内容      |
| `[msg]`           | 与 `[content]` 类似，消息内容 |
| `[org_content]`   | 原始短信或通知内容            |
| `[timestamp]`     | 当前毫秒时间戳                |
| `[device_mark]`   | 通用设置里的设备名称          |
| `[app_version]`   | SmsForwarder 版本             |
| `[card_slot]`     | 短信卡槽 / App 通知标题       |
| `[title]`         | 短信卡槽 / App 通知标题       |
| `[receive_time]`  | 手机收到短信或通知的时间      |

## 6. 简洁消息页面和字段抽取 API

本项目提供一个轻量消息页面和 JSON API。前端不做临时清洗；页面展示的是后端已经入库的抽取字段。

| 能力         | 地址                      | 说明                                                                              |
| ------------ | ------------------------- | --------------------------------------------------------------------------------- |
| 消息页面     | `GET /messages`           | 使用 Pico CSS + Go `html/template` 服务端渲染，展示来源筛选、关键词搜索和消息卡片 |
| 消息列表 API | `GET /api/messages`       | 支持 `source`、`q`、`limit`、`offset`                                             |
| 消息统计 API | `GET /api/messages/stats` | 返回 `total`、`bySource`                                                          |

### 6.1 当前数据可抽取字段

基于当前 PostgreSQL 中真实聊天数据审计：QQ 351 条、微信 69 条、飞书 12 条。三类 App 通知稳定具备以下原始字段：

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

后端会持久化这些抽取字段：

| 扩展字段             | 用途                                                     |
| -------------------- | -------------------------------------------------------- |
| `conversation_title` | 会话标题；QQ 会去除 `(N条新消息)` / `（N条新消息）` 后缀 |
| `clean_content`      | 清洗正文；QQ 会去掉正文首行中稳定可识别的作者前缀        |
| `processed_at`       | 后端加工/回填时间                                        |

真实数据里还零散出现链接、`@`、金额样式、数字码样式等内容特征；它们保留在正文中，不单独做成页面功能。

## 7. 服务端验证方法

### 7.1 健康检查

```bash
curl http://127.0.0.1:18088/healthz
```

期望返回：

```json
{ "status": "ok" }
```

### 7.2 验证页面和 API

```bash
curl -i 'http://127.0.0.1:18088/messages'
curl 'http://127.0.0.1:18088/api/messages?source=qq&limit=5'
curl 'http://127.0.0.1:18088/api/messages/stats'
```

期望：`/messages` 返回 `text/html` 并包含“消息列表”和 Pico CSS 链接；API 返回包含 `conversationTitle`、`cleanContent`、`processedAt` 的 JSON。

### 7.3 发送一条无 source 短信模拟请求

```bash
curl -i -X POST http://127.0.0.1:18088/webhook/smsforwarder \
  -H 'Content-Type: application/json' \
  -d '{
    "sender":"+8613800138000",
    "senderName":"中国移动",
    "title":"SIM1",
    "content":"+8613800138000\n测试短信验证码 123456\nSIM1\nSubId：0",
    "body":"测试短信验证码 123456",
    "originalContent":"测试短信验证码 123456",
    "device":"backup-phone",
    "receiveTime":"2026-06-10 10:00:00",
    "timestamp":"1781056800000",
    "cardSlot":"SIM1",
    "appVersion":"3.5.0.260224"
  }'
```

期望 HTTP 状态：`202 Accepted`，数据库中 `source` 自动写为 `sms`，`content` 写为干净正文 `测试短信验证码 123456`，同时写入抽取字段。

### 7.4 使用项目内置 mock 一次性发送四类消息

```bash
docker compose exec smsforwarder-webhook /app/smsforwarder-mock \
  -url http://smsforwarder-webhook:8080/webhook/smsforwarder
```

mock 会发送 4 条无 `source` JSON 请求：短信、微信、飞书、QQ；服务端应自动识别四类来源。

### 7.5 查询 PostgreSQL 入库和 QQ 回填结果

```bash
docker exec postgresql sh -lc '
psql -U "$POSTGRES_USER" -d smsforwarder_messages -c "select id, source, conversation_title, left(clean_content, 80) as clean_content_prefix, processed_at from smsforwarder_messages order by id desc limit 20;"'
```

只看 QQ 清洗回填效果：

```bash
docker exec postgresql sh -lc '
psql -U "$POSTGRES_USER" -d smsforwarder_messages -c "select id, conversation_title, left(clean_content, 100) as clean_content_prefix, processed_at from smsforwarder_messages where source = '\''qq'\'' order by id desc limit 20;"'
```

## 8. 排错

- `400 Bad Request`：检查 JSON 是否合法，`sender/from` 是否足够让服务端识别类型，以及 `body/originalContent/message/content` 是否至少有一个非空。
- `/api/messages` 没有返回抽取字段：确认服务已重启并执行 `002_add_message_enrichment.sql`，再检查 `processed_at` 是否为空。
- QQ 清洗正文异常：检查通知正文首行是否为 `作者：正文` / `作者:正文` 格式；URL（例如 `https://example.com:443/path`）不会被误拆。
- `503 /healthz`：服务连不上 PostgreSQL，检查 `.env` 数据库配置和容器是否在 `1panel-network`。
- SmsForwarder 收到失败：确认 WebServer 地址能从手机访问；如果服务器在内网，需要手机和服务器在同一网络或配置公网反向代理。
- 微信/飞书/QQ 没转发：检查 Android 通知读取权限、SmsForwarder 后台保活、规则是否选中了对应 App；服务端识别依赖 `[from]` 中的 App 包名。
