CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_lweixin_text_page
    ON lweixin_text_message (conversation_id, received_at DESC, message_id DESC);
