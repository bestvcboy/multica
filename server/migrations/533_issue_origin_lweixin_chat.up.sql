-- Extend issue.origin_type for issues created by the Lweixin /issue
-- command. origin_id stores the chat_session id, matching the existing
-- Lark / Slack / DingTalk / WeCom / Telegram channel origins.
--
-- This only widens the allowed set: the CHECK is recreated NOT VALID so the
-- ACCESS EXCLUSIVE lock is brief. It stays unvalidated until a later
-- maintenance pass; validation of the old rows is unchanged.
ALTER TABLE issue DROP CONSTRAINT IF EXISTS issue_origin_type_check;
ALTER TABLE issue ADD CONSTRAINT issue_origin_type_check
    CHECK (origin_type IN ('autopilot', 'quick_create', 'lark_chat', 'slack_chat', 'agent_create', 'dingtalk_chat', 'wecom_chat', 'telegram_chat', 'lweixin_chat'))
    NOT VALID;
