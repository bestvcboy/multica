CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_lweixin_conversation_scope
    ON lweixin_conversation (workspace_id, installation_id, account_id, chat_type, chat_id);
