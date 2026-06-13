package webhook_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"smsforwarder-webhook/internal/webhook"
)

type recordingStore struct {
	mu       sync.Mutex
	messages []webhook.Message
	pingErr  error
}

func (s *recordingStore) SaveMessage(_ context.Context, msg webhook.Message) (int64, error) {
	webhook.EnrichMessage(&msg)

	s.mu.Lock()
	defer s.mu.Unlock()

	msg.ID = int64(len(s.messages) + 1)
	s.messages = append(s.messages, msg)
	return msg.ID, nil
}

func (s *recordingStore) ListMessages(_ context.Context, query webhook.MessageQuery) ([]webhook.Message, error) {
	query = webhook.NormalizeMessageQuery(query)

	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := make([]webhook.Message, 0, len(s.messages))
	for _, msg := range s.messages {
		if query.Source != "" && msg.Source != query.Source {
			continue
		}
		if query.Search != "" {
			haystack := strings.ToLower(strings.Join([]string{
				msg.ConversationTitle,
				msg.CleanContent,
				msg.Title,
				msg.SenderName,
			}, " "))
			if !strings.Contains(haystack, strings.ToLower(query.Search)) {
				continue
			}
		}
		filtered = append(filtered, msg)
	}

	if query.Offset >= len(filtered) {
		return []webhook.Message{}, nil
	}
	end := query.Offset + query.Limit
	if end > len(filtered) {
		end = len(filtered)
	}
	out := make([]webhook.Message, end-query.Offset)
	copy(out, filtered[query.Offset:end])
	return out, nil
}

func (s *recordingStore) MessageStats(ctx context.Context, query webhook.MessageQuery) (webhook.MessageStats, error) {
	messages, err := s.ListMessages(ctx, query)
	if err != nil {
		return webhook.MessageStats{}, err
	}

	stats := webhook.MessageStats{Total: int64(len(messages))}
	sourceCounts := make(map[string]int64)
	for _, msg := range messages {
		sourceCounts[msg.Source]++
	}
	stats.BySource = messageCounts(sourceCounts)
	return stats, nil
}

func (s *recordingStore) Ping(_ context.Context) error {
	return s.pingErr
}

func (s *recordingStore) all() []webhook.Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]webhook.Message, len(s.messages))
	copy(out, s.messages)
	return out
}

func messageCounts(values map[string]int64) []webhook.MessageCount {
	counts := make([]webhook.MessageCount, 0, len(values))
	for name, count := range values {
		counts = append(counts, webhook.MessageCount{Name: name, Count: count})
	}
	return counts
}

func hasCount(counts []webhook.MessageCount, name string, want int64) bool {
	for _, count := range counts {
		if count.Name == name && count.Count == want {
			return true
		}
	}
	return false
}

func assertForbiddenAbsent(t *testing.T, body string) {
	t.Helper()
	for _, forbidden := range []string{
		"suggestedCategory",
		"suggestedPriority",
		"suggestedAction",
		"suggestedTags",
		"readStatus",
		"read-status",
		"ai_tools",
		"标为已读",
		"标为未读",
		"归档",
		"建议分类",
		"最低优先级",
		"阅读状态",
		"conversationKind",
		"messageAuthor",
		"byConversationKind",
		"appPackage",
		"sign",
		"system",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("body contains forbidden feature marker %q:\n%s", forbidden, body)
		}
	}
}

func TestServerInfersSourcesWithoutSourceField(t *testing.T) {
	store := &recordingStore{}
	server := webhook.NewServer(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	handler := server.Routes()

	samples := []struct {
		wantSource string
		sender     string
		title      string
		content    string
		body       string
	}{
		{wantSource: "sms", sender: "13055442609", title: "SIM1_", content: "13055442609\n短信验证码 123456\nSIM1_\nSubId：0\n2026-06-10 16:39:50\nOnePlus", body: "短信验证码 123456"},
		{wantSource: "wechat", sender: "com.tencent.mm", title: "微信好友", content: "com.tencent.mm\n微信消息内容\n微信好友\nUID：0\n2026-06-10 16:37:52\nOnePlus", body: "微信消息内容"},
		{wantSource: "feishu", sender: "com.ss.android.lark", title: "飞书群", content: "com.ss.android.lark\n飞书消息内容\n飞书群\nUID：0\n2026-06-10 16:38:54\nOnePlus", body: "飞书消息内容"},
		{wantSource: "qq", sender: "com.tencent.mobileqq", title: "QQ好友", content: "com.tencent.mobileqq\nQQ消息内容\nQQ好友\nUID：0\n2026-06-10 16:39:27\nOnePlus", body: "QQ消息内容"},
	}

	for _, sample := range samples {
		payload := map[string]any{
			"sender":          sample.sender,
			"senderName":      sample.title,
			"title":           sample.title,
			"content":         sample.content,
			"originalContent": sample.body,
			"device":          "OnePlus",
			"receiveTime":     "2026-06-10 10:00:00",
			"timestamp":       "1781056800000",
			"cardSlot":        sample.title,
			"appVersion":      "3.5.0.260224",
		}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/webhook/smsforwarder", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusAccepted {
			t.Fatalf("%s: expected status %d, got %d with body %s", sample.wantSource, http.StatusAccepted, rr.Code, rr.Body.String())
		}
	}

	messages := store.all()
	if len(messages) != len(samples) {
		t.Fatalf("expected %d stored messages, got %d", len(samples), len(messages))
	}
	for i, sample := range samples {
		msg := messages[i]
		if msg.Source != sample.wantSource {
			t.Errorf("message %d source = %q, want %q", i, msg.Source, sample.wantSource)
		}
		if msg.Sender != sample.sender {
			t.Errorf("message %d sender = %q, want %q", i, msg.Sender, sample.sender)
		}
		if msg.Content != sample.body {
			t.Errorf("message %d content = %q, want clean body %q", i, msg.Content, sample.body)
		}
		if msg.OriginalContent != sample.body {
			t.Errorf("message %d original content = %q, want %q", i, msg.OriginalContent, sample.body)
		}
		if msg.ForwarderTimestamp == nil || *msg.ForwarderTimestamp != 1781056800000 {
			t.Errorf("message %d forwarder timestamp = %v, want 1781056800000", i, msg.ForwarderTimestamp)
		}
		if len(msg.RawPayload) == 0 {
			t.Errorf("message %d raw payload was not stored", i)
		}
	}
}

func TestServerPrefersExplicitBodyField(t *testing.T) {
	store := &recordingStore{}
	server := webhook.NewServer(store, slog.New(slog.NewTextHandler(io.Discard, nil)))

	body := []byte(`{"sender":"13055442609","title":"SIM1_","content":"13055442609\n元信息污染内容\nSIM1_\nSubId：0","body":"干净正文","deviceMark":"OnePlus"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/smsforwarder", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	server.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusAccepted, rr.Code, rr.Body.String())
	}
	messages := store.all()
	if len(messages) != 1 {
		t.Fatalf("expected 1 stored message, got %d", len(messages))
	}
	if got := messages[0].Content; got != "干净正文" {
		t.Fatalf("content = %q, want explicit body", got)
	}
	if got := messages[0].Device; got != "OnePlus" {
		t.Fatalf("device = %q, want deviceMark alias", got)
	}
}

func TestServerIgnoresStaleSourceWhenItCanInfer(t *testing.T) {
	store := &recordingStore{}
	server := webhook.NewServer(store, slog.New(slog.NewTextHandler(io.Discard, nil)))

	body := []byte(`{"source":"qq","sender":"19999999999","title":"SIM1_测试运营商_18888888888","content":"19999999999\n短信正文\nSIM1_测试运营商_18888888888\nSubId：0","originalContent":"短信正文"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/smsforwarder", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	server.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusAccepted, rr.Code, rr.Body.String())
	}
	messages := store.all()
	if got := messages[0].Source; got != "sms" {
		t.Fatalf("source = %q, want inferred sms instead of stale source", got)
	}
}

func TestServerRejectsInvalidMessages(t *testing.T) {
	store := &recordingStore{}
	server := webhook.NewServer(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	handler := server.Routes()

	tests := []struct {
		name string
		body string
	}{
		{name: "unsupported source without inference", body: `{"source":"email","content":"hello"}`},
		{name: "unknown app package without source", body: `{"sender":"com.example.unknown","content":"hello"}`},
		{name: "missing content", body: `{"sender":"+8613800138000"}`},
		{name: "invalid json", body: `{not-json`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/webhook/smsforwarder", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d with body %s", http.StatusBadRequest, rr.Code, rr.Body.String())
			}
		})
	}

	if got := len(store.all()); got != 0 {
		t.Fatalf("expected no stored messages after invalid requests, got %d", got)
	}
}

func TestServerHealthz(t *testing.T) {
	store := &recordingStore{}
	server := webhook.NewServer(store, slog.New(slog.NewTextHandler(io.Discard, nil)))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	server.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected JSON content type, got %q", got)
	}
}

func TestServerMessagesAPIAndStatsExposeExtractedFieldsOnly(t *testing.T) {
	store := &recordingStore{}
	mustSaveMessage(t, store, webhook.Message{
		Source:  "qq",
		Sender:  "com.tencent.mobileqq",
		Title:   "AI 交流群(2条新消息)",
		Content: "小明：Claude API 额度还有吗？",
	})
	mustSaveMessage(t, store, webhook.Message{
		Source:  "qq",
		Sender:  "com.tencent.mobileqq",
		Title:   "闲聊群",
		Content: "张三：早上好",
	})
	mustSaveMessage(t, store, webhook.Message{
		Source:  "wechat",
		Sender:  "com.tencent.mm",
		Title:   "微信好友",
		Content: "微信通知",
	})

	server := webhook.NewServer(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	handler := server.Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/messages?source=qq&category=ai_tools&priority=4&status=unread&q=Claude", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected messages API status %d, got %d with body %s", http.StatusOK, rr.Code, rr.Body.String())
	}
	assertForbiddenAbsent(t, rr.Body.String())

	var listResponse struct {
		Query    webhook.MessageQuery `json:"query"`
		Messages []webhook.Message    `json:"messages"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&listResponse); err != nil {
		t.Fatalf("decode messages API response: %v", err)
	}
	if len(listResponse.Messages) != 1 {
		t.Fatalf("expected 1 filtered message, got %d: %#v", len(listResponse.Messages), listResponse.Messages)
	}
	msg := listResponse.Messages[0]
	if msg.ConversationTitle != "AI 交流群" || msg.CleanContent != "Claude API 额度还有吗？" {
		t.Fatalf("unexpected extracted API message: %#v", msg)
	}
	if listResponse.Query.Source != "qq" || listResponse.Query.Search != "Claude" {
		t.Fatalf("unexpected normalized query: %#v", listResponse.Query)
	}

	statsReq := httptest.NewRequest(http.MethodGet, "/api/messages/stats?source=qq", nil)
	statsRR := httptest.NewRecorder()
	handler.ServeHTTP(statsRR, statsReq)
	if statsRR.Code != http.StatusOK {
		t.Fatalf("expected stats API status %d, got %d with body %s", http.StatusOK, statsRR.Code, statsRR.Body.String())
	}
	assertForbiddenAbsent(t, statsRR.Body.String())
	var stats webhook.MessageStats
	if err := json.NewDecoder(statsRR.Body).Decode(&stats); err != nil {
		t.Fatalf("decode stats API response: %v", err)
	}
	if stats.Total != 2 {
		t.Fatalf("stats total = %d, want 2", stats.Total)
	}
	if !hasCount(stats.BySource, "qq", 2) {
		t.Fatalf("unexpected stats: %#v", stats)
	}

	statusReq := httptest.NewRequest(http.MethodPost, "/api/messages/1/read-status", strings.NewReader(`{"readStatus":"read"}`))
	statusReq.Header.Set("Content-Type", "application/json")
	statusRR := httptest.NewRecorder()
	handler.ServeHTTP(statusRR, statusReq)
	if statusRR.Code != http.StatusNotFound {
		t.Fatalf("read-status API should be removed, got status %d with body %s", statusRR.Code, statusRR.Body.String())
	}
}

func TestServerMessagesPageRendersSimpleExtractedFieldUI(t *testing.T) {
	store := &recordingStore{}
	mustSaveMessage(t, store, webhook.Message{
		Source:  "qq",
		Sender:  "com.tencent.mobileqq",
		Title:   "AI 交流群(2条新消息)",
		Content: "小明：Claude API 额度还有吗？",
	})

	server := webhook.NewServer(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/messages?source=qq&category=ai_tools&priority=4&status=unread&q=Claude+API", nil)
	server.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected messages page status %d, got %d with body %s", http.StatusOK, rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("expected HTML content type, got %q", got)
	}
	body := rr.Body.String()
	assertForbiddenAbsent(t, body)
	for _, want := range []string{
		"消息列表",
		"https://cdn.jsdelivr.net/npm/@picocss/pico@2/css/pico.min.css",
		"AI 交流群",
		"Claude API 额度还有吗？",
		"会话：AI 交流群",
		"/api/messages?q=Claude&#43;API&amp;source=qq",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("messages page body missing %q:\n%s", want, body)
		}
	}
}

func mustSaveMessage(t *testing.T, store *recordingStore, msg webhook.Message) webhook.Message {
	t.Helper()
	id, err := store.SaveMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("save message: %v", err)
	}
	messages := store.all()
	for _, saved := range messages {
		if saved.ID == id {
			return saved
		}
	}
	t.Fatalf("saved message id %d not found", id)
	return webhook.Message{}
}
