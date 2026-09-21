package lweixin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/multica-ai/multica/server/internal/integrations/channel"
	"github.com/multica-ai/multica/server/internal/integrations/channel/engine"
	"github.com/multica-ai/multica/server/internal/util"
	pgt "github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const (
	msgAgentOffline     = "\u26a0\ufe0f The agent is offline right now. Your message was received and will be handled once it is back online."
	msgAgentArchivedMsg = "\U0001F4CC This agent is archived. Its chats remain readable, but it no longer answers here."
	msgFreshPending     = "\u2705 Fresh start ready. Your next chat message will run without previous context."
	msgChatStarted      = "\u2705 Started a new Multica chat. Your next message will enter it."
	msgIssueUsage       = "Please include an issue title. Use:\n\n/issue <title>\n[description] (optional)"
	msgIssueNotMember   = "You are not a member of this Multica workspace, so I cannot file an issue for you. Ask a workspace admin to invite you, then send the command again."
	msgIssueDisabled    = "This LWEIXIN bot is not connected to Multica (or was disconnected). Ask a workspace admin to reconnect it."
	msgBindingGroupHint = "Please message me in a direct chat first, then link your Multica account."
)

// bindingMinter is the binding-token surface the replier needs.
type bindingMinter interface {
	Mint(ctx context.Context, wsID, instID pgt.UUID, lweixinUserID string) (BindingToken, error)
}

// OutboundReplier delivers verdict replies (binding prompt, offline / archived
// notice, control + issue confirmations) into the originating chat. Mirrors
// telegram/replier.go minus threading, editing, and typing.
type OutboundReplier struct {
	binding     bindingMinter
	decrypt     Decrypter
	appURL      string
	bindingPath string
	client      *http.Client
	logger      *slog.Logger
}

// OutboundReplierConfig configures the replier. Binding + AppURL are required
// for the NeedsBinding prompt; without them the prompt is skipped (other
// notices still fire).
type OutboundReplierConfig struct {
	Binding     bindingMinter
	Decrypt     Decrypter
	AppURL      string
	BindingPath string
	HTTPClient  *http.Client
	Logger      *slog.Logger
}

var _ engine.OutboundReplier = (*OutboundReplier)(nil)

// NewOutboundReplier builds the replier.
func NewOutboundReplier(cfg OutboundReplierConfig) *OutboundReplier {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	bp := cfg.BindingPath
	if bp == "" {
		bp = "/lweixin/bind"
	}
	if !strings.HasPrefix(bp, "/") {
		bp = "/" + bp
	}
	return &OutboundReplier{
		binding:     cfg.Binding,
		decrypt:     cfg.Decrypt,
		appURL:      strings.TrimRight(cfg.AppURL, "/"),
		bindingPath: bp,
		client:      cfg.HTTPClient,
		logger:      logger,
	}
}

// Reply routes each outcome to its user-visible message. Errors are logged,
// never propagated: the replier runs detached from the inbound ACK path.
func (r *OutboundReplier) Reply(ctx context.Context, inst engine.ResolvedInstallation, msg channel.InboundMessage, res engine.Result) {
	switch res.Outcome {
	case engine.OutcomeNeedsBinding:
		if err := r.sendBindingPrompt(ctx, inst, msg, res); err != nil {
			r.warn(ctx, inst, "binding prompt", err)
		}
	case engine.OutcomeAgentOffline:
		r.tryPost(ctx, inst, msg, msgAgentOffline, "offline notice")
	case engine.OutcomeAgentArchived:
		r.tryPost(ctx, inst, msg, msgAgentArchivedMsg, "archived notice")
	case engine.OutcomeFreshPending:
		r.tryPost(ctx, inst, msg, msgFreshPending, "fresh-start confirmation")
	case engine.OutcomeChatStarted:
		r.tryPost(ctx, inst, msg, msgChatStarted, "new-chat confirmation")
	case engine.OutcomeIssueUsage:
		r.tryPost(ctx, inst, msg, msgIssueUsage, "issue usage reply")
	case engine.OutcomeIngested:
		if res.IssueID.Valid {
			text := issueCreatedText(res)
			if res.IssueDuplicate {
				text = issueDuplicateText(res)
			}
			r.tryPost(ctx, inst, msg, text, "issue outcome reply")
		}
	case engine.OutcomeDropped:
		if text := droppedReplyText(res, msg); text != "" {
			r.tryPost(ctx, inst, msg, text, "drop refusal")
		}
	}
}

func (r *OutboundReplier) warn(ctx context.Context, inst engine.ResolvedInstallation, what string, err error) {
	r.logger.WarnContext(ctx, "lweixin replier: "+what+" failed",
		"installation_id", util.UUIDToString(inst.ID), "error", err)
}

func (r *OutboundReplier) tryPost(ctx context.Context, inst engine.ResolvedInstallation, msg channel.InboundMessage, text, what string) {
	if err := r.post(ctx, inst, msg, text); err != nil {
		r.warn(ctx, inst, what, err)
	}
}

func (r *OutboundReplier) sendBindingPrompt(ctx context.Context, inst engine.ResolvedInstallation, msg channel.InboundMessage, res engine.Result) error {
	// A group-visible bearer link could be redeemed by another member and
	// bind the sender to the wrong user; ask for a direct chat, mirroring
	// the Telegram / Slack group policy.
	if msg.Source.ChatType == channel.ChatTypeGroup {
		return r.post(ctx, inst, msg, msgBindingGroupHint)
	}
	sender := res.Sender
	if sender == "" {
		sender = msg.Source.SenderID
	}
	if sender == "" {
		return errors.New("missing sender id")
	}
	if r.binding == nil {
		return errors.New("binding service not configured")
	}
	if r.appURL == "" {
		return errors.New("app url not configured")
	}
	tok, err := r.binding.Mint(ctx, inst.WorkspaceID, inst.ID, sender)
	if err != nil {
		return fmt.Errorf("mint binding token: %w", err)
	}
	link := r.appURL + r.bindingPath + "?token=" + url.QueryEscape(tok.Raw)
	text := "Hi! To start chatting with me, link your account to Multica:\n" + link +
		"\n(This link expires in 15 minutes.)"
	return r.post(ctx, inst, msg, text)
}

// post resolves the installation credentials from the carried platform row
// and sends plain text back into the originating chat.
func (r *OutboundReplier) post(ctx context.Context, inst engine.ResolvedInstallation, msg channel.InboundMessage, text string) error {
	row, ok := inst.Platform.(db.ChannelInstallation)
	if !ok {
		return errors.New("installation platform row unavailable")
	}
	creds, err := decodeCredentials(row.Config, r.decrypt)
	if err != nil {
		return fmt.Errorf("decode credentials: %w", err)
	}
	if msg.Source.ChatID == "" {
		return errors.New("empty chat id")
	}
	api := newAPI(creds.BaseURL, creds.APIToken, r.client)
	if _, err := api.sendText(ctx, msg.Source.ChatID, text); err != nil {
		return fmt.Errorf("post lweixin reply: %w", err)
	}
	return nil
}

func issueCreatedText(res engine.Result) string {
	id := issueResultIdentifier(res)
	if strings.TrimSpace(res.IssueTitle) == "" {
		return "\u2705 Created " + id
	}
	return "\u2705 Created " + id + " \u2014 " + res.IssueTitle
}

func issueDuplicateText(res engine.Result) string {
	id := issueResultIdentifier(res)
	if strings.TrimSpace(res.IssueTitle) == "" {
		return "\u26a0\ufe0f Not created \u2014 active issue " + id + " already exists."
	}
	return "\u26a0\ufe0f Not created \u2014 active issue " + id + " already exists: " + res.IssueTitle
}

func issueResultIdentifier(res engine.Result) string {
	if res.IssueIdentifier != "" {
		return res.IssueIdentifier
	}
	if res.IssueNumber > 0 {
		return fmt.Sprintf("#%d", res.IssueNumber)
	}
	return util.UUIDToString(res.IssueID)
}

// isAddressedIssueCommand reports an explicit /issue command; only those
// earn a drop refusal, mirroring telegram.
func isAddressedIssueCommand(msg channel.InboundMessage) bool {
	if !msg.AddressedToBot {
		return false
	}
	source := msg.CommandText
	if source == "" {
		source = msg.Text
	}
	_, ok := engine.ParseIssueCommand(source)
	return ok
}

func droppedReplyText(res engine.Result, msg channel.InboundMessage) string {
	if !isAddressedIssueCommand(msg) {
		return ""
	}
	switch res.DropReason {
	case engine.DropReasonNonWorkspaceMember:
		return msgIssueNotMember
	case engine.DropReasonRevokedInstallation:
		return msgIssueDisabled
	default:
		return ""
	}
}
