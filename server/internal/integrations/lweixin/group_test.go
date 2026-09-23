package lweixin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/integrations/channel"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

func TestGroupPullPreservesLegacyEnvelopeAndSilentHandler(t *testing.T) {
	sends := 0
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/messages":
			_, _ = w.Write([]byte(`{"data":{"messages":[{"seq":1,"new_msg_id":"m1","type":1,"content":"hello @bot","is_group":true,"group_sender":"member","from":"room@chatroom","to":"bot"}]}}`))
		case "/api/message/send":
			sends++
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()
	id := dbid.NewV7()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	received := 0
	check := func(msg channel.InboundMessage) {
		t.Helper()
		if msg.Source.ChatType != channel.ChatTypeGroup || msg.Source.ChatID != "room@chatroom" ||
			msg.Source.SenderID != "member" || !msg.AddressedToBot || msg.SkipAgentRun {
			t.Errorf("group envelope changed: %+v", msg)
		}
	}
	factory := newLweixinFactory(ChannelDeps{SilentHandler: func(got pgtype.UUID) channel.InboundHandler {
		if got != id {
			t.Fatalf("installation: %v", got)
		}
		return func(_ context.Context, msg channel.InboundMessage) error {
			received++
			check(msg)
			cancel()
			return nil
		}
	}})
	raw, _ := json.Marshal(installConfig{AppID: "bot", BaseURL: gateway.URL, SilentReceive: true})
	ch, err := factory(channel.Config{ID: id, Raw: raw, Handler: func(context.Context, channel.InboundMessage) error {
		t.Error("silent installation entered shared router")
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	if received != 1 || sends != 0 {
		t.Fatalf("received=%d sends=%d", received, sends)
	}

	legacyCtx, cancelLegacy := context.WithCancel(context.Background())
	defer cancelLegacy()
	legacyReceived := 0
	raw, _ = json.Marshal(installConfig{AppID: "bot", BaseURL: gateway.URL})
	legacy, err := factory(channel.Config{ID: id, Raw: raw, Handler: func(_ context.Context, msg channel.InboundMessage) error {
		legacyReceived++
		check(msg)
		cancelLegacy()
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Connect(legacyCtx); err != nil {
		t.Fatal(err)
	}
	if legacyReceived != 1 || sends != 0 {
		t.Fatalf("legacy received=%d sends=%d", legacyReceived, sends)
	}
}

func TestGroupTriggerReason(t *testing.T) {
	for _, tc := range []struct {
		chatType, trigger, reason string
	}{
		{"group", "mention", "mention_metadata_unavailable"},
		{"group", "all", "execution_isolation_unavailable"},
		{"p2p", "mention", ""},
	} {
		route := ConversationRoute{Conversation: Conversation{ChatType: tc.chatType}}
		setGroupTrigger(&route, tc.trigger)
		if (tc.chatType == "group" && route.TriggerMode != tc.trigger) ||
			route.TriggerReason != tc.reason {
			t.Errorf("%s/%s: %+v", tc.chatType, tc.trigger, route)
		}
	}
}
