DROP INDEX IF EXISTS idx_smsforwarder_messages_category_created_at;
DROP INDEX IF EXISTS idx_smsforwarder_messages_read_status_created_at;
DROP INDEX IF EXISTS idx_smsforwarder_messages_suggested_tags;

ALTER TABLE smsforwarder_messages
    DROP COLUMN IF EXISTS suggested_category,
    DROP COLUMN IF EXISTS suggested_priority,
    DROP COLUMN IF EXISTS suggested_action,
    DROP COLUMN IF EXISTS suggested_tags,
    DROP COLUMN IF EXISTS read_status;
