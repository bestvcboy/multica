CREATE TABLE IF NOT EXISTS lweixin_conversation (
    id UUID NOT NULL,
    workspace_id UUID NOT NULL,
    installation_id UUID NOT NULL,
    account_id TEXT NOT NULL,
    chat_type TEXT NOT NULL CHECK (chat_type IN ('p2p', 'group')),
    chat_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_message_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS lweixin_text_message (
    conversation_id UUID NOT NULL,
    message_id TEXT NOT NULL,
    sender_id TEXT NOT NULL,
    text TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
