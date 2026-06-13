DROP INDEX IF EXISTS idx_smsforwarder_messages_conversation_kind_created_at;
DROP INDEX IF EXISTS idx_smsforwarder_messages_message_kind_created_at;

ALTER TABLE smsforwarder_messages
    DROP COLUMN IF EXISTS sign,
    DROP COLUMN IF EXISTS app_package,
    DROP COLUMN IF EXISTS conversation_kind,
    DROP COLUMN IF EXISTS message_author,
    DROP COLUMN IF EXISTS message_kind;
