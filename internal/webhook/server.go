package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

const sourceInferenceError = "message source could not be inferred; set sender/from to [from] with a supported sender or package value"

var allowedSources = map[string]struct{}{
	"sms":    {},
	"wechat": {},
	"feishu": {},
	"qq":     {},
}

type Message struct {
	ID                 int64           `json:"id"`
	Source             string          `json:"source"`
	Sender             string          `json:"sender,omitempty"`
	SenderName         string          `json:"senderName,omitempty"`
	Title              string          `json:"title,omitempty"`
	Content            string          `json:"content"`
	OriginalContent    string          `json:"originalContent,omitempty"`
	Device             string          `json:"device,omitempty"`
	ReceiveTime        string          `json:"receiveTime,omitempty"`
	ForwarderTimestamp *int64          `json:"timestamp,omitempty"`
	CardSlot           string          `json:"cardSlot,omitempty"`
	AppVersion         string          `json:"appVersion,omitempty"`
	RawPayload         json.RawMessage `json:"rawPayload,omitempty"`
	ConversationTitle  string          `json:"conversationTitle,omitempty"`
	CleanContent       string          `json:"cleanContent,omitempty"`
	CreatedAt          string          `json:"createdAt,omitempty"`
	ProcessedAt        string          `json:"processedAt,omitempty"`
}
type Store interface {
	SaveMessage(context.Context, Message) (int64, error)
	ListMessages(context.Context, MessageQuery) ([]Message, error)
	MessageStats(context.Context, MessageQuery) (MessageStats, error)
	Ping(context.Context) error
}

type Server struct {
	store  Store
	logger *slog.Logger
}

type incomingPayload struct {
	// Source is accepted only for backward compatibility. New SmsForwarder
	// templates should omit it; the service infers source from sender/from so
	// phone-side rules do not need per-source JSON bodies.
	Source string `json:"source"`

	Sender          string         `json:"sender"`
	From            string         `json:"from"`
	SenderName      string         `json:"senderName"`
	Name            string         `json:"name"`
	Title           string         `json:"title"`
	Content         string         `json:"content"`
	Body            string         `json:"body"`
	Message         string         `json:"message"`
	RawContent      string         `json:"rawContent"`
	OriginalContent string         `json:"originalContent"`
	Device          string         `json:"device"`
	DeviceMark      string         `json:"deviceMark"`
	ReceiveTime     string         `json:"receiveTime"`
	Time            string         `json:"time"`
	Timestamp       string         `json:"timestamp"`
	CardSlot        string         `json:"cardSlot"`
	AppVersion      string         `json:"appVersion"`
	Extra           map[string]any `json:"extra,omitempty"`
}

func NewServer(store Store, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	return &Server{store: store, logger: logger}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/messages", s.handleMessagesPage)
	mux.HandleFunc("/api/messages", s.handleMessagesAPI)
	mux.HandleFunc("/api/messages/stats", s.handleMessageStatsAPI)
	mux.HandleFunc("/webhook/smsforwarder", s.handleSmsForwarder)
	return mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if err := s.store.Ping(r.Context()); err != nil {
		s.logger.Error("health check failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "error",
			"error":  "database unavailable",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSmsForwarder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	defer r.Body.Close()

	rawPayload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload is too large or unreadable"})
		return
	}

	var payload incomingPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json payload"})
		return
	}

	msg, err := normalizePayload(payload, rawPayload)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	id, err := s.store.SaveMessage(r.Context(), msg)
	if err != nil {
		s.logger.Error("failed to save message", "error", err, "source", msg.Source)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store message"})
		return
	}

	s.logger.Info("message stored", "id", id, "source", msg.Source)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status": "accepted",
		"id":     id,
	})
}

func normalizePayload(payload incomingPayload, rawPayload []byte) (Message, error) {
	content, originalContent, err := normalizeContent(payload)
	if err != nil {
		return Message{}, err
	}

	sender := firstNonEmpty(payload.Sender, payload.From)
	title := strings.TrimSpace(payload.Title)
	source := normalizeSource(payload, sender, title, content)
	if source == "" {
		return Message{}, errors.New(sourceInferenceError)
	}
	if _, ok := allowedSources[source]; !ok {
		return Message{}, errors.New(sourceInferenceError)
	}

	forwarderTimestamp, err := parseTimestamp(payload.Timestamp)
	if err != nil {
		return Message{}, err
	}

	senderName := firstNonEmpty(payload.SenderName, payload.Name, title)

	msg := Message{
		Source:             source,
		Sender:             strings.TrimSpace(sender),
		SenderName:         senderName,
		Title:              title,
		Content:            content,
		OriginalContent:    originalContent,
		Device:             firstNonEmpty(payload.Device, payload.DeviceMark),
		ReceiveTime:        firstNonEmpty(payload.ReceiveTime, payload.Time),
		ForwarderTimestamp: forwarderTimestamp,
		CardSlot:           strings.TrimSpace(payload.CardSlot),
		AppVersion:         strings.TrimSpace(payload.AppVersion),
		RawPayload:         append(json.RawMessage(nil), rawPayload...),
	}
	EnrichMessage(&msg)
	return msg, nil
}

func normalizeContent(payload incomingPayload) (string, string, error) {
	body := firstNonEmpty(payload.Body, payload.RawContent)
	message := strings.TrimSpace(payload.Message)
	content := strings.TrimSpace(payload.Content)
	originalContent := strings.TrimSpace(payload.OriginalContent)

	switch {
	case body != "":
		if originalContent == "" {
			originalContent = body
		}
		return body, originalContent, nil
	case originalContent != "":
		return originalContent, originalContent, nil
	case message != "":
		return message, message, nil
	case content != "":
		return content, content, nil
	default:
		return "", "", errors.New("content is required")
	}
}

func normalizeSource(payload incomingPayload, sender, title, content string) string {
	if inferred := inferSource(sender, title, content); inferred != "" {
		return inferred
	}

	// Legacy fallback only. Automatic inference intentionally wins so a stale
	// phone-side source value cannot misclassify SMS as QQ/WeChat/etc.
	return strings.ToLower(strings.TrimSpace(payload.Source))
}

func inferSource(sender, title, content string) string {
	packageHint := strings.ToLower(strings.TrimSpace(sender))

	switch {
	case strings.Contains(packageHint, "com.tencent.mm"):
		return "wechat"
	case strings.Contains(packageHint, "lark") || strings.Contains(packageHint, "feishu"):
		return "feishu"
	case strings.Contains(packageHint, "com.tencent.mobileqq") || strings.Contains(packageHint, "mobileqq"):
		return "qq"
	case looksLikeSMSSender(sender) || looksLikeSMSMetadata(title, content):
		return "sms"
	default:
		return ""
	}
}

func looksLikeSMSSender(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && !looksLikeAndroidPackage(trimmed)
}

func looksLikeAndroidPackage(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || !strings.Contains(trimmed, ".") || trimmed != strings.ToLower(trimmed) {
		return false
	}
	if strings.ContainsAny(trimmed, " \t\n\r:/") {
		return false
	}

	parts := strings.Split(trimmed, ".")
	if len(parts) < 2 {
		return false
	}

	hasLetter := false
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			switch {
			case r >= 'a' && r <= 'z':
				hasLetter = true
			case r >= '0' && r <= '9':
			case r == '_':
			default:
				return false
			}
		}
	}
	return hasLetter
}

func looksLikeSMSMetadata(title, content string) bool {
	upperTitle := strings.ToUpper(strings.TrimSpace(title))
	lowerContent := strings.ToLower(strings.TrimSpace(content))

	return strings.Contains(upperTitle, "SIM") ||
		strings.Contains(lowerContent, "subid") ||
		strings.Contains(content, "卡槽")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func parseTimestamp(value string) (*int64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}

	parsed, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return nil, errors.New("timestamp must be a millisecond unix timestamp")
	}
	return &parsed, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, `{"error":"failed to encode response"}`, http.StatusInternalServerError)
	}
}
