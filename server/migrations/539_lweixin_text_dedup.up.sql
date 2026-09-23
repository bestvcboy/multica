CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_lweixin_text_dedup
    ON lweixin_text_message (conversation_id, message_id);
