CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_lweixin_conversation_id
    ON lweixin_conversation (id);
