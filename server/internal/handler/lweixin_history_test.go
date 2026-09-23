package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/integrations/channel"
	"github.com/multica-ai/multica/server/internal/integrations/lweixin"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/internal/util/secretbox"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

func TestLweixinRegisterSilentDefaultPreservesLegacyReconnect(t *testing.T) {
	ctx := context.Background()
	ws, err := util.ParseUUID(testWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	user, err := util.ParseUUID(testUserID)
	if err != nil {
		t.Fatal(err)
	}
	var agent pgtype.UUID
	if err := testPool.QueryRow(ctx, `SELECT id FROM agent WHERE workspace_id = $1 LIMIT 1`, ws).Scan(&agent); err != nil {
		t.Fatal(err)
	}
	box, err := secretbox.New(make([]byte, secretbox.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := lweixin.NewInstallService(testHandler.Queries, testPool, box, nil)
	if err != nil {
		t.Fatal(err)
	}
	params := lweixin.RegisterParams{
		WorkspaceID: ws, AgentID: agent, InitiatorID: user,
		AppID: "history-default-" + util.UUIDToString(dbid.NewV7()), BaseURL: "http://unused.invalid",
	}
	inst, err := svc.Register(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM channel_installation WHERE id = $1`, inst.ID)
	})
	var config map[string]any
	if err := json.Unmarshal(inst.Config, &config); err != nil || config["silent_receive"] != true {
		t.Fatalf("new installation is not silent: %s %v", inst.Config, err)
	}
	_, err = testPool.Exec(ctx, `UPDATE channel_installation SET config = config - 'silent_receive' WHERE id = $1`, inst.ID)
	if err != nil {
		t.Fatal(err)
	}
	reconnected, err := svc.Register(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if reconnected.ID != inst.ID {
		t.Fatal("reconnect changed installation identity")
	}
	if err := json.Unmarshal(reconnected.Config, &config); err != nil {
		t.Fatal(err)
	}
	if config["silent_receive"] == true {
		t.Fatalf("legacy connection silently switched to silent: %s", reconnected.Config)
	}
}

func TestLweixinHistoryAdminScope(t *testing.T) {
	ctx := context.Background()
	ws, err := util.ParseUUID(testWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	otherWS, inst, member := dbid.NewV7(), dbid.NewV7(), dbid.NewV7()
	suffix := util.UUIDToString(inst)
	_, err = testPool.Exec(ctx, `INSERT INTO workspace (id, name, slug) VALUES ($1, 'LWEIXIN history test', $2)`,
		otherWS, "lweixin-api-"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	_, err = testPool.Exec(ctx, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`,
		otherWS, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = testPool.Exec(ctx, `INSERT INTO "user" (id, name, email) VALUES ($1, 'LWEIXIN member', $2)`,
		member, "lweixin-member-"+suffix+"@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	_, err = testPool.Exec(ctx, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'member')`,
		ws, member)
	if err != nil {
		t.Fatal(err)
	}
	_, err = testPool.Exec(ctx, `INSERT INTO channel_installation
		(id, workspace_id, agent_id, channel_type, config, installer_user_id)
		VALUES ($1, $2, $3, 'lweixin', $4, $5)`,
		inst, ws, dbid.NewV7(), fmt.Sprintf(`{"app_id":"%s","silent_receive":true}`, suffix), testUserID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM lweixin_text_message WHERE conversation_id IN
			(SELECT id FROM lweixin_conversation WHERE installation_id = $1)`, inst)
		_, _ = testPool.Exec(ctx, `DELETE FROM lweixin_conversation WHERE installation_id = $1`, inst)
		_, _ = testPool.Exec(ctx, `DELETE FROM channel_installation WHERE id = $1`, inst)
		_, _ = testPool.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, otherWS)
		_, _ = testPool.Exec(ctx, `DELETE FROM member WHERE workspace_id = $1 AND user_id = $2`, ws, member)
		_, _ = testPool.Exec(ctx, `DELETE FROM "user" WHERE id = $1`, member)
	})

	history := lweixin.NewHistory(testPool)
	testHandler.LweixinHistory = history
	t.Cleanup(func() { testHandler.LweixinHistory = nil })
	msg := channel.InboundMessage{
		Type: channel.MsgTypeText, MessageID: "one", Text: "received",
		Raw: []byte(fmt.Sprintf(`{"bot_id":"%s"}`, suffix)),
		Source: channel.Source{ChannelType: lweixin.TypeLweixin, ChatType: channel.ChatTypeP2P,
			ChatID: "friend", SenderID: "friend"},
	}
	if err := history.SilentHandler(inst)(ctx, msg); err != nil {
		t.Fatal(err)
	}
	convs, err := history.ListConversations(ctx, ws, inst, 10, 0)
	if err != nil || len(convs) != 1 {
		t.Fatalf("fixture history: %+v %v", convs, err)
	}
	router := chi.NewRouter()
	router.Route("/api/workspaces/{id}", func(r chi.Router) {
		r.Use(middleware.RequireWorkspaceRoleFromURL(testHandler.Queries, "id", "owner", "admin"))
		r.Get("/lweixin/installations/{installationId}/conversations", testHandler.ListLweixinConversations)
		r.Get("/lweixin/installations/{installationId}/conversations/{conversationId}/messages", testHandler.ListLweixinMessages)
	})
	base := "/api/workspaces/" + testWorkspaceID + "/lweixin/installations/" + suffix + "/conversations"
	request := func(path, user string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-User-ID", user)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	if got := request(base, testUserID); got.Code != http.StatusOK {
		t.Fatalf("owner list: %d %s", got.Code, got.Body.String())
	}
	if got := request(base+"/"+convs[0].ID+"/messages?limit=1", testUserID); got.Code != http.StatusOK {
		t.Fatalf("owner history: %d %s", got.Code, got.Body.String())
	}
	if got := request(base, util.UUIDToString(member)); got.Code != http.StatusForbidden {
		t.Fatalf("member list: %d", got.Code)
	}
	if got := request(base+"/"+convs[0].ID+"/messages", util.UUIDToString(member)); got.Code != http.StatusForbidden {
		t.Fatalf("member history: %d", got.Code)
	}
	otherBase := "/api/workspaces/" + util.UUIDToString(otherWS) + "/lweixin/installations/" + suffix + "/conversations"
	if got := request(otherBase, testUserID); got.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace installation: %d", got.Code)
	}
	if got := request(otherBase+"/"+convs[0].ID+"/messages", testUserID); got.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace history: %d", got.Code)
	}
	if got := request(base+"?limit=1000", testUserID); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid pagination: %d", got.Code)
	}
}
