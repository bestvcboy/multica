package lweixin

import (
	"encoding/json"
	"strconv"

	"github.com/multica-ai/multica/server/internal/integrations/channel"
	"github.com/multica-ai/multica/server/internal/integrations/channel/engine"
)

// lweixinRawEvent carries the LWEIXIN-specific fields absent from the
// cross-platform envelope; read back only inside the LWEIXIN resolvers.
type lweixinRawEvent struct {
	// BotID routes the message to its installation (config app_id); it is
	// the bot account wxid, mirrored from the config at build time.
	BotID string `json:"bot_id"`
	// EventType is a coarse label for drop audits.
	EventType string `json:"event_type"`
	// GroupSender is the individual member wxid inside a group chat.
	GroupSender string `json:"group_sender,omitempty"`
}

// msgTypeText is the LWEIXIN text type (server.py: 3 image, 34 voice,
// 43 video, 47 emoji, 49 file/appmsg). Only text enters v1; the rest just
// advance the cursor.
const msgTypeText = 1

// inboundFromMessage normalizes one pulled message. ok=false means the row
// must not reach the core: self echoes, non-text bodies, empty payloads.
func inboundFromMessage(m inboundMessage, selfWxid string) (channel.InboundMessage, bool) {
	if m.Type != msgTypeText || m.Content == "" {
		return channel.InboundMessage{}, false
	}
	// p2p rows key the peer in From with the account on To; group rows key
	// the room id in From and ride the sender in GroupSender.
	sender, chatID := m.From, m.From
	if m.IsGroup {
		sender = m.GroupSender
	} else if sender == "" || sender == selfWxid {
		sender, chatID = m.To, m.To
	}
	if sender == "" || sender == selfWxid || chatID == "" {
		return channel.InboundMessage{}, false
	}

	chatType := channel.ChatTypeP2P
	if m.IsGroup {
		chatType = channel.ChatTypeGroup
	}
	// The pull API carries no mention metadata, so every group row is
	// conservatively treated as addressed; group sessions handle the rest.
	addressed := true

	text := m.Content
	commandText := text
	forceFresh := false
	if control, ok := engine.ParseControlCommand(text); ok {
		text = control.Body
		forceFresh = control.Kind == engine.ControlCommandFreshSession
	}

	eventID := m.NewMsgID
	if eventID == "" {
		eventID = m.MsgID
	}
	if eventID == "" {
		eventID = strconv.FormatInt(m.Seq, 10)
	}

	raw, _ := json.Marshal(lweixinRawEvent{
		BotID:       selfWxid,
		EventType:   "message",
		GroupSender: m.GroupSender,
	})

	return channel.InboundMessage{
		EventID:        eventID,
		MessageID:      eventID,
		Type:           channel.MsgTypeText,
		Text:           text,
		CommandText:    commandText,
		AddressedToBot: addressed,
		ForceFresh:     forceFresh,
		Source: channel.Source{
			ChannelType:    TypeLweixin,
			ChatID:         chatID,
			ChatType:       chatType,
			SenderID:       sender,
			SenderStableID: sender,
		},
		Raw: raw,
	}, true
}
