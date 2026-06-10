package smsforwarder_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"smsforwarder-webhook/internal/smsforwarder"
)

func TestSamplesCoverSmsWechatFeishuAndQQ(t *testing.T) {
	samples := smsforwarder.Samples("backup-phone")
	if len(samples) != 4 {
		t.Fatalf("expected 4 sample messages, got %d", len(samples))
	}

	gotSources := make([]string, 0, len(samples))
	for _, sample := range samples {
		gotSources = append(gotSources, sample.Source)
		if sample.Device != "backup-phone" {
			t.Fatalf("sample %s device = %q, want backup-phone", sample.Source, sample.Device)
		}
		if sample.Content == "" {
			t.Fatalf("sample %s content is empty", sample.Source)
		}
		if sample.Body == "" {
			t.Fatalf("sample %s body is empty", sample.Source)
		}
		if sample.Timestamp == "" {
			t.Fatalf("sample %s timestamp is empty", sample.Source)
		}

		body, err := json.Marshal(sample)
		if err != nil {
			t.Fatalf("sample %s is not valid JSON: %v", sample.Source, err)
		}
		var raw map[string]any
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatalf("sample %s marshaled JSON cannot be decoded: %v", sample.Source, err)
		}
		if _, ok := raw["source"]; ok {
			t.Fatalf("sample %s JSON should not include source: %s", sample.Source, string(body))
		}
	}

	sort.Strings(gotSources)
	wantSources := []string{"feishu", "qq", "sms", "wechat"}
	for i := range wantSources {
		if gotSources[i] != wantSources[i] {
			t.Fatalf("sources = %v, want %v", gotSources, wantSources)
		}
	}
}

func TestSendPostsSamplesAsSmsForwarderJSONWithoutSource(t *testing.T) {
	received := make([]map[string]any, 0, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		received = append(received, payload)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	samples := smsforwarder.Samples("test-phone")
	if err := smsforwarder.Send(context.Background(), server.Client(), server.URL, samples); err != nil {
		t.Fatalf("send samples: %v", err)
	}

	if len(received) != len(samples) {
		t.Fatalf("received %d samples, want %d", len(received), len(samples))
	}
	for i, payload := range received {
		if _, ok := payload["source"]; ok {
			t.Fatalf("received sample %d unexpectedly includes source: %#v", i, payload)
		}
		if payload["body"] == "" {
			t.Fatalf("received sample %d has empty body: %#v", i, payload)
		}
	}
}
