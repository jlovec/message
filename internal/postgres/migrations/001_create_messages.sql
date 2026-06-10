CREATE TABLE IF NOT EXISTS smsforwarder_messages (
    id BIGSERIAL PRIMARY KEY,
    source TEXT NOT NULL CHECK (source IN ('sms', 'wechat', 'feishu', 'qq')),
    sender TEXT NOT NULL DEFAULT '',
    sender_name TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL,
    original_content TEXT NOT NULL DEFAULT '',
    device TEXT NOT NULL DEFAULT '',
    receive_time TEXT NOT NULL DEFAULT '',
    forwarder_timestamp_millis BIGINT,
    sign TEXT NOT NULL DEFAULT '',
    app_package TEXT NOT NULL DEFAULT '',
    card_slot TEXT NOT NULL DEFAULT '',
    app_version TEXT NOT NULL DEFAULT '',
    raw_payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_smsforwarder_messages_source_created_at
    ON smsforwarder_messages (source, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_smsforwarder_messages_sender_created_at
    ON smsforwarder_messages (sender, created_at DESC);
