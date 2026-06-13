ALTER TABLE smsforwarder_messages
    ADD COLUMN IF NOT EXISTS conversation_title TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS clean_content TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS processed_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_smsforwarder_messages_conversation_created_at
    ON smsforwarder_messages (conversation_title, created_at DESC);
