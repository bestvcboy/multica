package lweixin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/server/internal/integrations/channel"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

func TestSilentFactoryReceivesWithoutSharedRouterOrSend(t *testing.T) {
	sends := 0
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/messages":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"messages":[{"seq":1,"new_msg_id":"m1","type":1,"content":"hello","from":"friend","to":"bot"}]}}`))
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
	factory := newLweixinFactory(ChannelDeps{SilentHandler: func(gotID pgtype.UUID) channel.InboundHandler {
		if gotID != id {
			t.Errorf("installation mismatch")
		}
		return func(_ context.Context, msg channel.InboundMessage) error {
			received++
			if msg.Source.ChatID != "friend" || msg.CommandText != "hello" {
				t.Errorf("unexpected inbound: %+v", msg)
			}
			cancel()
			return nil
		}
	}})
	raw, _ := json.Marshal(installConfig{AppID: "bot", BaseURL: gateway.URL, SilentReceive: true})
	ch, err := factory(channel.Config{ID: id, Raw: raw, Handler: func(context.Context, channel.InboundMessage) error {
		return errors.New("shared member router was invoked")
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

	raw, _ = json.Marshal(installConfig{AppID: "bot", BaseURL: gateway.URL})
	legacy, err := factory(channel.Config{ID: id, Raw: raw, Handler: func(context.Context, channel.InboundMessage) error {
		return errors.New("legacy handler")
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Connect(context.Background()); err == nil || err.Error() != "legacy handler" {
		t.Fatalf("legacy route was changed: %v", err)
	}
}

func TestSilentHistoryScopesAndDeduplicates(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("managed DATABASE_URL not available")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	h := NewHistory(pool)
	ws, otherWS := dbid.NewV7(), dbid.NewV7()
	inst, otherInst := dbid.NewV7(), dbid.NewV7()
	for i, id := range []pgtype.UUID{ws, otherWS} {
		_, err := pool.Exec(ctx, `INSERT INTO workspace (id, name, slug) VALUES ($1, 'LWEIXIN test', $2)`,
			id, fmt.Sprintf("lweixin-history-%s-%d", util.UUIDToString(ws), i))
		if err != nil {
			t.Fatal(err)
		}
	}
	for i, row := range []struct{ id, workspace pgtype.UUID }{{inst, ws}, {otherInst, otherWS}} {
		_, err := pool.Exec(ctx, `INSERT INTO channel_installation
			(id, workspace_id, agent_id, channel_type, config, installer_user_id)
			VALUES ($1, $2, $3, 'lweixin', $4, $5)`,
			row.id, row.workspace, dbid.NewV7(),
			fmt.Sprintf(`{"app_id":"history-test-%s-%d","silent_receive":true}`, util.UUIDToString(inst), i), dbid.NewV7())
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM lweixin_text_message WHERE conversation_id IN
			(SELECT id FROM lweixin_conversation WHERE installation_id IN ($1, $2))`, inst, otherInst)
		_, _ = pool.Exec(ctx, `DELETE FROM lweixin_conversation WHERE installation_id IN ($1, $2)`, inst, otherInst)
		_, _ = pool.Exec(ctx, `DELETE FROM channel_installation WHERE id IN ($1, $2)`, inst, otherInst)
		_, _ = pool.Exec(ctx, `DELETE FROM workspace WHERE id IN ($1, $2)`, ws, otherWS)
	})
	msg := channel.InboundMessage{
		Type: channel.MsgTypeText, MessageID: "same-id", Text: "hello", CommandText: "hello",
		Raw: []byte(fmt.Sprintf(`{"bot_id":"history-test-%s-0"}`, util.UUIDToString(inst))),
		Source: channel.Source{ChannelType: TypeLweixin, ChatType: channel.ChatTypeP2P,
			ChatID: "friend", SenderID: "friend"},
	}
	other := msg
	other.Raw = []byte(fmt.Sprintf(`{"bot_id":"history-test-%s-1"}`, util.UUIDToString(inst)))
	for _, id := range []pgtype.UUID{inst, inst, otherInst} {
		current := msg
		if id == otherInst {
			current = other
		}
		if err := h.SilentHandler(id)(ctx, current); err != nil {
			t.Fatal(err)
		}
	}
	msg.Source.ChatType, msg.Source.ChatID, msg.Source.SenderID = channel.ChatTypeGroup, "room", "member"
	if err := h.SilentHandler(inst)(ctx, msg); err != nil {
		t.Fatal(err)
	}
	convs, err := h.ListConversations(ctx, ws, inst, 10, 0)
	if err != nil || len(convs) != 2 {
		t.Fatalf("conversations=%+v error=%v", convs, err)
	}
	for _, conv := range convs {
		cid, err := util.ParseUUID(conv.ID)
		if err != nil {
			t.Fatal(err)
		}
		messages, err := h.ListMessages(ctx, ws, inst, cid, 1, 0)
		if err != nil || len(messages) != 1 || messages[0].Text != "hello" {
			t.Fatalf("history=%+v error=%v", messages, err)
		}
		messages, err = h.ListMessages(ctx, ws, inst, cid, 1, 1)
		if err != nil || len(messages) != 0 {
			t.Fatalf("duplicate or pagination failure: %+v %v", messages, err)
		}
		if _, err := h.ListMessages(ctx, otherWS, otherInst, cid, 10, 0); !errors.Is(err, ErrHistoryNotFound) {
			t.Fatalf("cross-workspace history: %v", err)
		}
	}
	if _, err := h.ListConversations(ctx, otherWS, inst, 10, 0); !errors.Is(err, ErrHistoryNotFound) {
		t.Fatalf("cross-workspace installation: %v", err)
	}
	// A bot swap can retain the installation ID; the new account must not
	// reuse the prior bot's conversation for the same platform chat ID.
	swapped := msg
	swapped.MessageID = "after-swap"
	swapped.Raw = []byte(`{"bot_id":"replacement"}`)
	if _, err := pool.Exec(ctx, `UPDATE channel_installation
		SET config = jsonb_set(config, '{app_id}', '"replacement"') WHERE id = $1`, inst); err != nil {
		t.Fatal(err)
	}
	if err := h.SilentHandler(inst)(ctx, msg); err == nil {
		t.Fatal("stale bot poll accepted after account swap")
	}
	if err := h.SilentHandler(inst)(ctx, swapped); err != nil {
		t.Fatal(err)
	}
	convs, err = h.ListConversations(ctx, ws, inst, 10, 0)
	if err != nil || len(convs) != 3 {
		t.Fatalf("account-swap conversations=%+v error=%v", convs, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE channel_installation SET config = jsonb_set(config, '{silent_receive}', 'false')
		WHERE id = $1`, inst); err != nil {
		t.Fatal(err)
	}
	if err := h.SilentHandler(inst)(ctx, swapped); err == nil {
		t.Fatal("stale silent poll accepted after mode changed")
	}
	if _, err := pool.Exec(ctx, `UPDATE channel_installation
		SET config = jsonb_set(config, '{silent_receive}', 'true'), status = 'revoked'
		WHERE id = $1`, inst); err != nil {
		t.Fatal(err)
	}
	if err := h.SilentHandler(inst)(ctx, swapped); !errors.Is(err, ErrHistoryNotFound) {
		t.Fatalf("revoked installation accepted: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE channel_installation SET status = 'active' WHERE id = $1`, inst); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO lweixin_routing_policy (workspace_id, installation_id, account_id)
		VALUES ($1, $2, 'replacement')`, ws, inst); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT id FROM workspace WHERE id = $1 FOR UPDATE`, ws); err != nil {
		t.Fatal(err)
	}
	late := swapped
	late.MessageID = "late"
	done := make(chan error, 1)
	go func() { done <- h.SilentHandler(inst)(ctx, late) }()
	select {
	case err := <-done:
		t.Fatalf("inbound did not wait for workspace delete: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := DeleteWorkspaceHistory(ctx, tx, ws); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, ws); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("inbound recreated history after workspace deletion")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stale inbound did not finish after workspace deletion")
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM lweixin_conversation WHERE workspace_id = $1`, ws).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("workspace history remained: %d %v", remaining, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM lweixin_routing_policy WHERE workspace_id = $1`, ws).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("workspace routing remained: %d %v", remaining, err)
	}
}
