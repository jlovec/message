package webhook

import (
	"embed"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
)

//go:embed templates/messages.html
var messageTemplateFS embed.FS

var messagePageTemplate = template.Must(template.ParseFS(messageTemplateFS, "templates/messages.html"))

type messagesPageData struct {
	Query    MessageQuery
	Stats    MessageStats
	Messages []Message
	Sources  []string
	APIURL   string
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	http.Redirect(w, r, "/messages", http.StatusFound)
}

func (s *Server) handleMessagesPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	query := parseMessageQuery(r.URL.Query())
	messages, err := s.store.ListMessages(r.Context(), query)
	if err != nil {
		s.logger.Error("failed to list messages for page", "error", err)
		http.Error(w, "failed to list messages", http.StatusInternalServerError)
		return
	}
	stats, err := s.store.MessageStats(r.Context(), query)
	if err != nil {
		s.logger.Error("failed to load message stats for page", "error", err)
		http.Error(w, "failed to load message stats", http.StatusInternalServerError)
		return
	}

	data := messagesPageData{
		Query:    query,
		Stats:    stats,
		Messages: messages,
		Sources:  []string{"", "qq", "wechat", "feishu", "sms"},
		APIURL:   messagesAPIPath(query),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := messagePageTemplate.ExecuteTemplate(w, "messages.html", data); err != nil {
		s.logger.Error("failed to render messages page", "error", err)
	}
}

func (s *Server) handleMessagesAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	query := parseMessageQuery(r.URL.Query())
	messages, err := s.store.ListMessages(r.Context(), query)
	if err != nil {
		s.logger.Error("failed to list messages", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list messages"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"query":    query,
		"messages": messages,
	})
}

func (s *Server) handleMessageStatsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	query := parseMessageQuery(r.URL.Query())
	stats, err := s.store.MessageStats(r.Context(), query)
	if err != nil {
		s.logger.Error("failed to load message stats", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load message stats"})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func parseMessageQuery(values url.Values) MessageQuery {
	query := MessageQuery{
		Source: values.Get("source"),
		Search: values.Get("q"),
	}
	if limit, err := strconv.Atoi(values.Get("limit")); err == nil {
		query.Limit = limit
	}
	if offset, err := strconv.Atoi(values.Get("offset")); err == nil {
		query.Offset = offset
	}
	return NormalizeMessageQuery(query)
}

func messagesAPIPath(query MessageQuery) string {
	query = NormalizeMessageQuery(query)
	values := url.Values{}
	if query.Source != "" {
		values.Set("source", query.Source)
	}
	if query.Search != "" {
		values.Set("q", query.Search)
	}
	if values.Encode() == "" {
		return "/api/messages"
	}
	return "/api/messages?" + values.Encode()
}
