package smsforwarder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Payload struct {
	// Source is kept only as the expected server-side inference result for tests
	// and log messages. It is intentionally not sent to the webhook JSON body.
	Source          string         `json:"-"`
	Sender          string         `json:"sender"`
	SenderName      string         `json:"senderName"`
	Title           string         `json:"title"`
	Content         string         `json:"content"`
	Body            string         `json:"body"`
	OriginalContent string         `json:"originalContent"`
	Device          string         `json:"device"`
	ReceiveTime     string         `json:"receiveTime"`
	Timestamp       string         `json:"timestamp"`
	CardSlot        string         `json:"cardSlot"`
	AppVersion      string         `json:"appVersion"`
	Extra           map[string]any `json:"extra,omitempty"`
}

func Samples(device string) []Payload {
	device = strings.TrimSpace(device)
	if device == "" {
		device = "backup-phone"
	}

	now := time.Now()
	receiveTime := now.Format("2006-01-02 15:04:05")
	timestamp := fmt.Sprintf("%d", now.UnixMilli())

	return []Payload{
		{
			Source:          "sms",
			Sender:          "+8613800138000",
			SenderName:      "中国移动",
			Title:           "SIM1",
			Content:         "+8613800138000\n【Mock短信】验证码 123456，5 分钟内有效。\nSIM1\nSubId：0\n" + receiveTime + "\n" + device,
			Body:            "【Mock短信】验证码 123456，5 分钟内有效。",
			OriginalContent: "【Mock短信】验证码 123456，5 分钟内有效。",
			Device:          device,
			ReceiveTime:     receiveTime,
			Timestamp:       timestamp,
			CardSlot:        "SIM1",
			AppVersion:      "3.5.0.260224",
			Extra:           map[string]any{"mock": true, "channel": "sms"},
		},
		{
			Source:          "wechat",
			Sender:          "com.tencent.mm",
			SenderName:      "微信好友",
			Title:           "微信好友",
			Content:         "com.tencent.mm\n【Mock微信】这是一条微信通知转发消息。\n微信好友\nUID：0\n" + receiveTime + "\n" + device,
			Body:            "【Mock微信】这是一条微信通知转发消息。",
			OriginalContent: "【Mock微信】这是一条微信通知转发消息。",
			Device:          device,
			ReceiveTime:     receiveTime,
			Timestamp:       timestamp,
			CardSlot:        "微信好友",
			AppVersion:      "3.5.0.260224",
			Extra:           map[string]any{"mock": true, "channel": "wechat"},
		},
		{
			Source:          "feishu",
			Sender:          "com.ss.android.lark",
			SenderName:      "飞书项目群",
			Title:           "飞书项目群",
			Content:         "com.ss.android.lark\n【Mock飞书】项目群收到一条待办提醒。\n飞书项目群\nUID：0\n" + receiveTime + "\n" + device,
			Body:            "【Mock飞书】项目群收到一条待办提醒。",
			OriginalContent: "【Mock飞书】项目群收到一条待办提醒。",
			Device:          device,
			ReceiveTime:     receiveTime,
			Timestamp:       timestamp,
			CardSlot:        "飞书项目群",
			AppVersion:      "3.5.0.260224",
			Extra:           map[string]any{"mock": true, "channel": "feishu"},
		},
		{
			Source:          "qq",
			Sender:          "com.tencent.mobileqq",
			SenderName:      "QQ好友",
			Title:           "QQ好友",
			Content:         "com.tencent.mobileqq\n【Mock QQ】这是一条 QQ 通知转发消息。\nQQ好友\nUID：0\n" + receiveTime + "\n" + device,
			Body:            "【Mock QQ】这是一条 QQ 通知转发消息。",
			OriginalContent: "【Mock QQ】这是一条 QQ 通知转发消息。",
			Device:          device,
			ReceiveTime:     receiveTime,
			Timestamp:       timestamp,
			CardSlot:        "QQ好友",
			AppVersion:      "3.5.0.260224",
			Extra:           map[string]any{"mock": true, "channel": "qq"},
		},
	}
}

func Send(ctx context.Context, client *http.Client, endpoint string, payloads []Payload) error {
	if client == nil {
		client = http.DefaultClient
	}
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}

	for _, payload := range payloads {
		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal %s payload: %w", payload.Source, err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create %s request: %w", payload.Source, err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "smsforwarder-webhook-mock/1.0")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("send %s payload: %w", payload.Source, err)
		}
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("send %s payload: unexpected status %s", payload.Source, resp.Status)
		}
	}

	return nil
}
