package lweixin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// Outbound delivers finished agent replies back to LWEIXIN: the outbound
// half of the round trip, mirroring telegram.Outbound reduced to its
// terminal form. LWEIXIN has no message-edit or stream API, so the full
// answer posts once on EventChatDone; task_message frames, delivery leases,
// and failure notices are skipped. ponytail: single-replica assumption;
// add a persisted delivery dedup if production ever runs multiple replicas.
type Outbound struct {
	q       outboundQueries
	decrypt Decrypter
	client  *http.Client
	logger  *slog.Logger
}

// outboundQueries is the generated-query slice the subscriber needs;
// *db.Queries satisfies it.
type outboundQueries interface {
	GetChannelTaskDelivery(ctx context.Context, taskID pgtype.UUID) (db.ChannelTaskDelivery, error)
	GetChannelInstallation(ctx context.Context, arg db.GetChannelInstallationParams) (db.ChannelInstallation, error)
}

// NewOutbound builds the outbound subscriber.
func NewOutbound(q outboundQueries, decrypt Decrypter, client *http.Client, logger *slog.Logger) *Outbound {
	if logger == nil {
		logger = slog.Default()
	}
	return &Outbound{q: q, decrypt: decrypt, client: client, logger: logger}
}

// Register subscribes to chat completion on the process bus.
func (o *Outbound) Register(bus *events.Bus) {
	bus.Subscribe(protocol.EventChatDone, o.handleChatDone)
}

func (o *Outbound) handleChatDone(e events.Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	content := chatDoneContent(e.Payload)
	if content == "" {
		return
	}
	taskID, ok := eventTaskID(e)
	if !ok {
		return
	}
	delivery, err := o.q.GetChannelTaskDelivery(ctx, taskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return // task did not originate from a channel chat
		}
		o.logger.WarnContext(ctx, "lweixin outbound: delivery lookup failed", "error", err)
		return
	}
	if delivery.ChannelType != string(TypeLweixin) {
		return
	}
	inst, err := o.q.GetChannelInstallation(ctx, db.GetChannelInstallationParams{
		ID:          delivery.InstallationID,
		ChannelType: string(TypeLweixin),
	})
	if err != nil {
		o.logger.WarnContext(ctx, "lweixin outbound: installation load failed", "error", err)
		return
	}
	if inst.Status != "active" {
		return // revoked between trigger and reply
	}
	creds, err := decodeCredentials(inst.Config, o.decrypt)
	if err != nil {
		o.logger.WarnContext(ctx, "lweixin outbound: decode credentials failed", "error", err)
		return
	}
	chatID := delivery.ChannelChatID
	if len(delivery.Config) > 0 {
		var bc lweixinBindingConfig
		if jsonErr := json.Unmarshal(delivery.Config, &bc); jsonErr == nil && bc.ChatID != "" {
			chatID = bc.ChatID
		}
	}
	if chatID == "" {
		return
	}
	if _, ferr := newAPI(creds.BaseURL, creds.APIToken, o.client).sendText(ctx, chatID, content); ferr != nil {
		o.logger.WarnContext(ctx, "lweixin outbound: send failed", "chat_id", chatID, "error", ferr)
	}
}

// eventTaskID extracts the task id from the event envelope or payload.
func eventTaskID(e events.Event) (pgtype.UUID, bool) {
	raw := e.TaskID
	if raw == "" {
		switch p := e.Payload.(type) {
		case protocol.ChatDonePayload:
			raw = p.TaskID
		case map[string]any:
			raw, _ = p["task_id"].(string)
		}
	}
	id, err := util.ParseUUID(raw)
	return id, err == nil && id.Valid
}

// chatDoneContent extracts the reply text from an EventChatDone payload.
func chatDoneContent(payload any) string {
	switch p := payload.(type) {
	case protocol.ChatDonePayload:
		return p.Content
	case map[string]any:
		if s, ok := p["content"].(string); ok {
			return s
		}
	}
	return ""
}
