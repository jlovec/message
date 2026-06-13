package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"smsforwarder-webhook/internal/webhook"
)

const messageSelectColumns = `
    id,
    source,
    sender,
    sender_name,
    title,
    content,
    original_content,
    device,
    receive_time,
    forwarder_timestamp_millis,
    card_slot,
    app_version,
    raw_payload::text,
    conversation_title,
    clean_content,
    to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
    to_char(processed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')`

func (s *Store) ListMessages(ctx context.Context, query webhook.MessageQuery) ([]webhook.Message, error) {
	query = webhook.NormalizeMessageQuery(query)
	where, args := messageWhere(query)
	args = append(args, query.Limit, query.Offset)

	sql := `
SELECT ` + messageSelectColumns + `
FROM smsforwarder_messages
` + where + `
ORDER BY created_at DESC, id DESC
LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("list smsforwarder messages: %w", err)
	}
	defer rows.Close()

	messages := make([]webhook.Message, 0, query.Limit)
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate smsforwarder messages: %w", err)
	}
	return messages, nil
}

func (s *Store) MessageStats(ctx context.Context, query webhook.MessageQuery) (webhook.MessageStats, error) {
	query = webhook.NormalizeMessageQuery(query)
	where, args := messageWhere(query)

	stats := webhook.MessageStats{}
	if err := s.pool.QueryRow(ctx, `
SELECT count(*)
FROM smsforwarder_messages
`+where, args...).Scan(&stats.Total); err != nil {
		return webhook.MessageStats{}, fmt.Errorf("count smsforwarder messages: %w", err)
	}

	var err error
	stats.BySource, err = s.countBy(ctx, "source", where, args)
	if err != nil {
		return webhook.MessageStats{}, err
	}

	return stats, nil
}

func (s *Store) countBy(ctx context.Context, expression, where string, args []any) ([]webhook.MessageCount, error) {
	rows, err := s.pool.Query(ctx, `
SELECT `+expression+` AS name, count(*)
FROM smsforwarder_messages
`+where+`
GROUP BY 1
ORDER BY count(*) DESC, name ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("count smsforwarder messages by %s: %w", expression, err)
	}
	defer rows.Close()

	counts := make([]webhook.MessageCount, 0)
	for rows.Next() {
		var count webhook.MessageCount
		if err := rows.Scan(&count.Name, &count.Count); err != nil {
			return nil, fmt.Errorf("scan smsforwarder message count: %w", err)
		}
		counts = append(counts, count)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate smsforwarder message counts: %w", err)
	}
	return counts, nil
}

func messageWhere(query webhook.MessageQuery) (string, []any) {
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 2)

	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	if query.Source != "" {
		add("source = $%d", query.Source)
	}
	if query.Search != "" {
		args = append(args, "%"+strings.ToLower(query.Search)+"%")
		placeholder := len(args)
		clauses = append(clauses, fmt.Sprintf("(lower(conversation_title) LIKE $%d OR lower(clean_content) LIKE $%d OR lower(title) LIKE $%d OR lower(sender_name) LIKE $%d)", placeholder, placeholder, placeholder, placeholder))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func (s *Store) BackfillMessageEnrichment(ctx context.Context) (int64, error) {
	rows, err := s.pool.Query(ctx, `
SELECT `+messageSelectColumns+`
FROM smsforwarder_messages
WHERE processed_at IS NULL OR clean_content = ''
ORDER BY id ASC`)
	if err != nil {
		return 0, fmt.Errorf("query smsforwarder message enrichment backfill: %w", err)
	}
	defer rows.Close()

	var updated int64
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return updated, err
		}
		webhook.EnrichMessage(&msg)
		if _, err := s.pool.Exec(ctx, `
UPDATE smsforwarder_messages
SET conversation_title = $2,
    clean_content = $3,
    processed_at = now()
WHERE id = $1`,
			msg.ID,
			msg.ConversationTitle,
			msg.CleanContent,
		); err != nil {
			return updated, fmt.Errorf("backfill smsforwarder message enrichment id %d: %w", msg.ID, err)
		}
		updated++
	}
	if err := rows.Err(); err != nil {
		return updated, fmt.Errorf("iterate smsforwarder message enrichment backfill: %w", err)
	}
	return updated, nil
}

type messageScanner interface {
	Scan(dest ...any) error
}

func scanMessage(row messageScanner) (webhook.Message, error) {
	var msg webhook.Message
	var rawPayload string
	var createdAt, processedAt *string

	if err := row.Scan(
		&msg.ID,
		&msg.Source,
		&msg.Sender,
		&msg.SenderName,
		&msg.Title,
		&msg.Content,
		&msg.OriginalContent,
		&msg.Device,
		&msg.ReceiveTime,
		&msg.ForwarderTimestamp,
		&msg.CardSlot,
		&msg.AppVersion,
		&rawPayload,
		&msg.ConversationTitle,
		&msg.CleanContent,
		&createdAt,
		&processedAt,
	); err != nil {
		return webhook.Message{}, fmt.Errorf("scan smsforwarder message: %w", err)
	}

	msg.RawPayload = append(json.RawMessage(nil), rawPayload...)
	if createdAt != nil {
		msg.CreatedAt = *createdAt
	}
	if processedAt != nil {
		msg.ProcessedAt = *processedAt
	}
	return msg, nil
}
