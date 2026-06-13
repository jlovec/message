package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"smsforwarder-webhook/internal/webhook"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	store := &Store{pool: pool}
	if err := store.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	if err := RunMigrations(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	if _, err := store.BackfillMessageEnrichment(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) SaveMessage(ctx context.Context, msg webhook.Message) (int64, error) {
	webhook.EnrichMessage(&msg)

	const query = `
INSERT INTO smsforwarder_messages (
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
    raw_payload,
    conversation_title,
    clean_content,
    processed_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    $9, $10, $11, $12, $13, $14, now()
)
RETURNING id`

	var id int64
	if err := s.pool.QueryRow(ctx, query,
		msg.Source,
		msg.Sender,
		msg.SenderName,
		msg.Title,
		msg.Content,
		msg.OriginalContent,
		msg.Device,
		msg.ReceiveTime,
		msg.ForwarderTimestamp,
		msg.CardSlot,
		msg.AppVersion,
		string(msg.RawPayload),
		msg.ConversationTitle,
		msg.CleanContent,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("insert smsforwarder message: %w", err)
	}

	return id, nil
}
