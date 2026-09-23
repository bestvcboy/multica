ALTER TABLE lweixin_conversation
    DROP CONSTRAINT IF EXISTS lweixin_group_trigger_only,
    DROP COLUMN IF EXISTS trigger_mode;
