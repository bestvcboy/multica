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

func TestGroupPullDoesNotAssumeMentionOrSend(t *testing.T) {
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
	factory := newLweixinFactory(ChannelDeps{SilentHandler: func(got pgtype.UUID) channel.InboundHandler {
		if got != id {
			t.Fatalf("installation: %v", got)
		}
		return func(_ context.Context, msg channel.InboundMessage) error {
			received++
			if msg.Source.ChatType != channel.ChatTypeGroup || msg.Source.ChatID != "room@chatroom" ||
				msg.Source.SenderID != "member" || msg.AddressedToBot || !msg.SkipAgentRun {
				t.Errorf("unsafe group message: %+v", msg)
			}
			cancel()
			return nil
		}
	}})
	raw, _ := json.Marshal(installConfig{AppID: "bot", BaseURL: gateway.URL, SilentReceive: true})
	ch, err := factory(channel.Config{ID: id, Raw: raw})
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	if received != 1 || sends != 0 {
		t.Fatalf("received=%d sends=%d", received, sends)
	}
}
