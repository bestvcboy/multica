ALTER TABLE lweixin_conversation
    ADD COLUMN trigger_mode TEXT NOT NULL DEFAULT 'mention'
        CHECK (trigger_mode IN ('mention', 'all')),
    ADD CONSTRAINT lweixin_group_trigger_only
        CHECK (chat_type = 'group' OR trigger_mode = 'mention');
