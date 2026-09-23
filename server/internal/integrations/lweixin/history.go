package lweixin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/server/internal/integrations/channel"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

var ErrHistoryNotFound = errors.New("lweixin conversation or installation not found")

type Conversation struct {
	ID            string    `json:"id"`
	AccountID     string    `json:"account_id"`
	ChatType      string    `json:"chat_type"`
	ChatID        string    `json:"chat_id"`
	LastMessageAt time.Time `json:"last_message_at"`
}

type TextMessage struct {
	MessageID  string    `json:"message_id"`
	SenderID   string    `json:"sender_id"`
	Text       string    `json:"text"`
	ReceivedAt time.Time `json:"received_at"`
}

// History is separate from agent chat sessions: neither an agent nor a
// Multica member is the owner of these inbound messages.
type History struct{ pool *pgxpool.Pool }

func NewHistory(pool *pgxpool.Pool) *History { return &History{pool: pool} }

func (h *History) SilentHandler(id pgtype.UUID) channel.InboundHandler {
	return func(ctx context.Context, msg channel.InboundMessage) error {
		if msg.Type != channel.MsgTypeText || msg.MessageID == "" || msg.Source.ChatID == "" ||
			msg.Source.SenderID == "" || msg.Source.ChannelType != TypeLweixin ||
			(msg.Source.ChatType != channel.ChatTypeP2P && msg.Source.ChatType != channel.ChatTypeGroup) {
			return errors.New("lweixin: invalid silent inbound message")
		}
		// CommandText retains the received text if the normalizer strips
		// a control-command prefix from Text.
		text := msg.CommandText
		if text == "" {
			text = msg.Text
		}
		if text == "" {
			return errors.New("lweixin: empty silent inbound text")
		}
		tx, err := h.pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		var wsID pgtype.UUID
		var config []byte
		err = tx.QueryRow(ctx, `SELECT workspace_id, config FROM channel_installation
			WHERE id = $1 AND channel_type = 'lweixin' AND status = 'active'`, id).Scan(&wsID, &config)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrHistoryNotFound
		}
		if err != nil {
			return err
		}
		var saved installConfig
		if err := json.Unmarshal(config, &saved); err != nil {
			return err
		}
		if !saved.SilentReceive {
			return errors.New("lweixin: installation no longer in silent mode")
		}
		raw, err := decodeLweixinRaw(msg)
		if err != nil || saved.AppID == "" || raw.BotID != saved.AppID {
			return errors.New("lweixin: inbound account does not match installation")
		}
		var locked pgtype.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM workspace WHERE id = $1 FOR KEY SHARE`, wsID).Scan(&locked); err != nil {
			return fmt.Errorf("lweixin workspace unavailable: %w", err)
		}
		var convID pgtype.UUID
		err = tx.QueryRow(ctx, `INSERT INTO lweixin_conversation
			(id, workspace_id, installation_id, account_id, chat_type, chat_id)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (workspace_id, installation_id, account_id, chat_type, chat_id)
			DO UPDATE SET chat_id = EXCLUDED.chat_id RETURNING id`,
			dbid.NewV7(), wsID, id, saved.AppID, string(msg.Source.ChatType), msg.Source.ChatID).Scan(&convID)
		if err != nil {
			return fmt.Errorf("ensure lweixin conversation: %w", err)
		}
		tag, err := tx.Exec(ctx, `INSERT INTO lweixin_text_message
			(conversation_id, message_id, sender_id, text)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (conversation_id, message_id) DO NOTHING`,
			convID, msg.MessageID, msg.Source.SenderID, text)
		if err != nil {
			return fmt.Errorf("save lweixin text: %w", err)
		}
		if tag.RowsAffected() != 0 {
			if _, err := tx.Exec(ctx, `UPDATE lweixin_conversation
				SET last_message_at = now() WHERE id = $1`, convID); err != nil {
				return err
			}
		}
		return tx.Commit(ctx)
	}
}

// DeleteWorkspaceHistory runs inside the workspace deletion transaction,
// after its FOR UPDATE lock has stopped new inbound history writes.
func DeleteWorkspaceHistory(ctx context.Context, tx pgx.Tx, wsID pgtype.UUID) error {
	if _, err := tx.Exec(ctx, `DELETE FROM lweixin_text_message WHERE conversation_id IN
		(SELECT id FROM lweixin_conversation WHERE workspace_id = $1)`, wsID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM lweixin_conversation WHERE workspace_id = $1`, wsID)
	return err
}

func (h *History) ListConversations(ctx context.Context, wsID, instID pgtype.UUID, limit, offset int) ([]Conversation, error) {
	var exists bool
	err := h.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM channel_installation
		WHERE id = $1 AND workspace_id = $2 AND channel_type = 'lweixin')`, instID, wsID).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrHistoryNotFound
	}
	rows, err := h.pool.Query(ctx, `SELECT id::text, account_id, chat_type, chat_id, last_message_at
		FROM lweixin_conversation WHERE workspace_id = $1 AND installation_id = $2
		ORDER BY last_message_at DESC, id DESC LIMIT $3 OFFSET $4`, wsID, instID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Conversation{}
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.AccountID, &c.ChatType, &c.ChatID, &c.LastMessageAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (h *History) ListMessages(ctx context.Context, wsID, instID, convID pgtype.UUID, limit, offset int) ([]TextMessage, error) {
	var exists bool
	err := h.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lweixin_conversation
		WHERE id = $1 AND workspace_id = $2 AND installation_id = $3)`, convID, wsID, instID).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrHistoryNotFound
	}
	rows, err := h.pool.Query(ctx, `SELECT message_id, sender_id, text, received_at
		FROM lweixin_text_message WHERE conversation_id = $1
		ORDER BY received_at DESC, message_id DESC LIMIT $2 OFFSET $3`, convID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TextMessage{}
	for rows.Next() {
		var m TextMessage
		if err := rows.Scan(&m.MessageID, &m.SenderID, &m.Text, &m.ReceivedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
