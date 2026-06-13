package webhook

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var qqUnreadSuffixPattern = regexp.MustCompile(`[（(][0-9]+条新消息[）)]$`)

type MessageQuery struct {
	Source string `json:"source,omitempty"`
	Search string `json:"q,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

type MessageCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type MessageStats struct {
	Total    int64          `json:"total"`
	BySource []MessageCount `json:"bySource"`
}

func NormalizeMessageQuery(query MessageQuery) MessageQuery {
	query.Source = strings.ToLower(strings.TrimSpace(query.Source))
	if _, ok := allowedSources[query.Source]; !ok {
		query.Source = ""
	}

	query.Search = strings.TrimSpace(query.Search)
	if query.Limit <= 0 {
		query.Limit = 50
	}
	if query.Limit > 200 {
		query.Limit = 200
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	return query
}

func EnrichMessage(msg *Message) {
	if msg == nil {
		return
	}

	msg.Source = strings.ToLower(strings.TrimSpace(msg.Source))
	msg.Content = strings.TrimSpace(msg.Content)
	msg.OriginalContent = strings.TrimSpace(msg.OriginalContent)
	msg.Title = strings.TrimSpace(msg.Title)
	msg.SenderName = strings.TrimSpace(msg.SenderName)
	msg.CardSlot = strings.TrimSpace(msg.CardSlot)

	conversationTitle := firstNonEmpty(msg.Title, msg.SenderName, msg.CardSlot, msg.Sender)
	cleanContent := firstNonEmpty(msg.Content, msg.OriginalContent)

	if msg.Source == "qq" {
		conversationTitle = normalizeQQConversationTitle(conversationTitle)
		if author, clean := splitQQAuthor(cleanContent); author != "" {
			cleanContent = clean
		}
	}

	if cleanContent == "" {
		cleanContent = firstNonEmpty(msg.OriginalContent, msg.Content)
	}

	msg.ConversationTitle = conversationTitle
	msg.CleanContent = cleanContent
}

func normalizeQQConversationTitle(title string) string {
	trimmed := strings.TrimSpace(title)
	for {
		next := strings.TrimSpace(qqUnreadSuffixPattern.ReplaceAllString(trimmed, ""))
		if next == trimmed {
			return next
		}
		trimmed = next
	}
}

func splitQQAuthor(content string) (string, string) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "", ""
	}

	firstLine := trimmed
	rest := ""
	if idx := strings.Index(trimmed, "\n"); idx >= 0 {
		firstLine = trimmed[:idx]
		rest = strings.TrimSpace(trimmed[idx+1:])
	}

	colonIndex, colonLen := firstColon(firstLine)
	if colonIndex <= 0 {
		return "", trimmed
	}

	author := strings.TrimSpace(firstLine[:colonIndex])
	if !looksLikeQQAuthor(author) {
		return "", trimmed
	}

	cleanFirstLine := strings.TrimSpace(firstLine[colonIndex+colonLen:])
	if rest == "" {
		return author, cleanFirstLine
	}
	if cleanFirstLine == "" {
		return author, rest
	}
	return author, cleanFirstLine + "\n" + rest
}

func firstColon(value string) (int, int) {
	ascii := strings.Index(value, ":")
	wide := strings.Index(value, "：")
	switch {
	case ascii < 0 && wide < 0:
		return -1, 0
	case ascii >= 0 && (wide < 0 || ascii < wide):
		return ascii, 1
	default:
		return wide, len("：")
	}
}

func looksLikeQQAuthor(author string) bool {
	trimmed := strings.TrimSpace(author)
	if trimmed == "" || utf8.RuneCountInString(trimmed) > 32 {
		return false
	}
	lower := strings.ToLower(trimmed)
	if lower == "http" || lower == "https" || strings.Contains(lower, "://") {
		return false
	}
	return !strings.ContainsAny(trimmed, "\n\r\t")
}
