# SmsForwarder 配置文档

本文档用于把 Android 手机上的 [SmsForwarder](https://github.com/pppscn/SmsForwarder) 配置为向本项目 webhook 转发短信与 App 通知（微信、飞书、QQ）。

> 本 webhook **无需认证**。如果你后续暴露到公网，建议放在反向代理后面，用 IP 白名单、Basic Auth 或随机路径做额外保护。

## 1. 服务端信息

部署后 webhook 地址为：

```text
http://<服务器IP或域名>:18088/webhook/smsforwarder
```

健康检查地址：

```text
http://<服务器IP或域名>:18088/healthz
```

本机测试地址：

```text
http://127.0.0.1:18088/webhook/smsforwarder
```

请求方式：

- Method：`POST`
- Content-Type：`application/json;charset=utf-8`
- 鉴权：无

## 2. 服务端接收 JSON 格式

手机端推荐使用**统一无 `source` 字段**的 JSON 模板。`source` 仍会作为服务端内部分类写入数据库，但不需要再在 SmsForwarder 的每条规则里手动配置。

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
  "sign": "[sign]",
  "cardSlot": "[card_slot]",
  "appVersion": "[app_version]"
}
```

字段说明：

| 字段                         | 必填     | 推荐来源         | 说明                                                                                      |
| ---------------------------- | -------- | ---------------- | ----------------------------------------------------------------------------------------- |
| `source`                     | 否       | 不填             | 仅兼容旧模板；新配置应省略。服务端会自动推断并持久化 `sms` / `wechat` / `feishu` / `qq`   |
| `sender` / `from`            | 推荐     | `[from]`         | 短信号码，或 App 通知包名；这是类型识别的主要依据                                         |
| `senderName` / `name`        | 否       | `[title]`        | 通知标题 / 发送方名称                                                                     |
| `title`                      | 否       | `[title]`        | 短信卡槽或 App 通知标题；可辅助识别短信 SIM/SubId 元信息                                  |
| `body` / `rawContent`        | 推荐     | `[org_content]`  | 干净正文；服务端优先写入数据库 `content`                                                  |
| `originalContent`            | 推荐     | `[org_content]`  | 原始干净正文；当 `body` 为空时作为入库正文                                                |
| `message`                    | 否       | `[msg]`          | 正文别名；优先级低于 `body/rawContent/originalContent`                                    |
| `content`                    | 条件必填 | `[content]`      | 转发内容；真实测试中可能包含号码/标题/UID/时间/设备等元信息，仅在没有更干净字段时兜底入库 |
| `device` / `deviceMark`      | 否       | `[device_mark]`  | SmsForwarder 通用设置里的设备名称                                                         |
| `receiveTime` / `time`       | 否       | `[receive_time]` | 手机端接收时间                                                                            |
| `timestamp`                  | 否       | `[timestamp]`    | SmsForwarder 发送时的毫秒时间戳                                                           |
| `sign`                       | 否       | `[sign]`         | 如果 SmsForwarder 配置了 secret，则可传入签名；本服务只存储不校验                         |
| `appPackage` / `packageName` | 否       | `[from]`         | App 通知包名；通常 `sender` 已足够，只有需要显式补充包名时再填                            |
| `cardSlot`                   | 否       | `[card_slot]`    | 短信/来电 SIM 卡槽，或 App 通知标题                                                       |
| `appVersion`                 | 否       | `[app_version]`  | SmsForwarder App 版本                                                                     |

正文入库优先级：`body` → `rawContent` → `originalContent` → `message` → `content`。

类型识别规则：

| 输入特征                                                                      | 服务端识别为      |
| ----------------------------------------------------------------------------- | ----------------- |
| `sender/from/appPackage/packageName` 包含 `com.tencent.mm`                    | `wechat`          |
| 包名包含 `lark` 或 `feishu`，例如 `com.ss.android.lark`                       | `feishu`          |
| 包名包含 `com.tencent.mobileqq` 或 `mobileqq`                                 | `qq`              |
| `sender/from` 是号码、非 Android 包名，或标题/正文包含 `SIM`、`SubId`、`卡槽` | `sms`             |
| 无法识别                                                                      | `400 Bad Request` |

## 3. SmsForwarder Webhook 通道配置

在 SmsForwarder 中新增一个 Webhook 发送通道，推荐配置：

```text
名称：message-webhook
请求方式：POST
WebServer：http://<服务器IP或域名>:18088/webhook/smsforwarder
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
| `[sign]`          | 配置 secret 后生成的签名      |
| `[device_mark]`   | 通用设置里的设备名称          |
| `[app_version]`   | SmsForwarder 版本             |
| `[card_slot]`     | 短信卡槽 / App 通知标题       |
| `[title]`         | 短信卡槽 / App 通知标题       |
| `[receive_time]`  | 手机收到短信或通知的时间      |

## 6. 服务端验证方法

### 6.1 健康检查

```bash
curl http://127.0.0.1:18088/healthz
```

期望返回：

```json
{ "status": "ok" }
```

### 6.2 发送一条无 source 短信模拟请求

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
    "sign":"",
    "cardSlot":"SIM1",
    "appVersion":"3.5.0.260224"
  }'
```

期望 HTTP 状态：`202 Accepted`，数据库中 `source` 自动写为 `sms`，`content` 写为干净正文 `测试短信验证码 123456`。

### 6.3 使用项目内置 mock 一次性发送四类消息

```bash
docker compose exec smsforwarder-webhook /app/smsforwarder-mock \
  -url http://smsforwarder-webhook:8080/webhook/smsforwarder
```

mock 会发送 4 条无 `source` JSON 请求：短信、微信、飞书、QQ；服务端应自动识别四类来源。

### 6.4 查询 PostgreSQL 入库结果

```bash
docker exec postgresql sh -lc '
psql -U "$POSTGRES_USER" -d smsforwarder_messages -c "select id, source, sender, title, content, raw_payload ? '\''source'\'' as raw_has_source, created_at from smsforwarder_messages order by id desc limit 20;"'
```

## 7. 排错

- `400 Bad Request`：检查 JSON 是否合法，`sender/from/appPackage/packageName` 是否足够让服务端识别类型，以及 `body/originalContent/message/content` 是否至少有一个非空。
- `503 /healthz`：服务连不上 PostgreSQL，检查 `.env` 数据库配置和容器是否在 `1panel-network`。
- SmsForwarder 收到失败：确认 WebServer 地址能从手机访问；如果服务器在内网，需要手机和服务器在同一网络或配置公网反向代理。
- 微信/飞书/QQ 没转发：检查 Android 通知读取权限、SmsForwarder 后台保活、规则是否选中了对应 App；服务端识别依赖 `[from]` 中的 App 包名。
