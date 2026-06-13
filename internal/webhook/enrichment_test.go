package webhook

import "testing"

func TestNormalizeQQConversationTitleRemovesUnreadSuffixes(t *testing.T) {
	tests := map[string]string{
		"AI 交流群(3条新消息)":     "AI 交流群",
		"售前群（12条新消息）":       "售前群",
		"普通好友":              "普通好友",
		"交流群(3条新消息)（2条新消息）": "交流群",
	}

	for input, want := range tests {
		if got := normalizeQQConversationTitle(input); got != want {
			t.Fatalf("normalizeQQConversationTitle(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSplitQQAuthor(t *testing.T) {
	author, clean := splitQQAuthor("小明：Claude API 额度还有吗？\n第二行")
	if author != "小明" {
		t.Fatalf("author = %q, want 小明", author)
	}
	if clean != "Claude API 额度还有吗？\n第二行" {
		t.Fatalf("clean = %q", clean)
	}

	author, clean = splitQQAuthor("https://example.com:443/path")
	if author != "" || clean != "https://example.com:443/path" {
		t.Fatalf("URL-like content should not be split, got author=%q clean=%q", author, clean)
	}
}

func TestEnrichQQMessageExtractsChatFieldsOnly(t *testing.T) {
	msg := Message{
		Source:  "qq",
		Sender:  "com.tencent.mobileqq",
		Title:   "AI 交流群(3条新消息)",
		Content: "小明：Claude API 额度还有吗？",
	}

	EnrichMessage(&msg)

	if msg.ConversationTitle != "AI 交流群" {
		t.Fatalf("conversation title = %q", msg.ConversationTitle)
	}
	if msg.CleanContent != "Claude API 额度还有吗？" {
		t.Fatalf("clean content = %q", msg.CleanContent)
	}
}

func TestEnrichQQMessageKeepsMediaMarkerInCleanContent(t *testing.T) {
	msg := Message{
		Source:  "qq",
		Title:   "朋友",
		Content: "张三：[图片]",
	}

	EnrichMessage(&msg)

	if msg.CleanContent != "[图片]" {
		t.Fatalf("clean content = %q, want [图片]", msg.CleanContent)
	}
}

func TestEnrichWelcomeGroupMessageKeepsCleanContent(t *testing.T) {
	msg := Message{
		Source:  "qq",
		Title:   "AI 交流群",
		Content: "管理员：欢迎加入本群",
	}

	EnrichMessage(&msg)

	if msg.CleanContent != "欢迎加入本群" {
		t.Fatalf("clean content = %q, want 欢迎加入本群", msg.CleanContent)
	}
}

func TestNormalizeMessageQueryClampsInvalidValues(t *testing.T) {
	query := NormalizeMessageQuery(MessageQuery{
		Source: "unknown",
		Limit:  500,
		Offset: -1,
		Search: "  Claude  ",
	})

	if query.Source != "" {
		t.Fatalf("source = %q, want empty", query.Source)
	}
	if query.Limit != 200 {
		t.Fatalf("limit = %d, want 200", query.Limit)
	}
	if query.Offset != 0 {
		t.Fatalf("offset = %d, want 0", query.Offset)
	}
	if query.Search != "Claude" {
		t.Fatalf("search = %q, want Claude", query.Search)
	}
}
