package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/integrations/channel"
	"github.com/multica-ai/multica/server/internal/integrations/lweixin"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

func TestLweixinRoutingAdminAndRevision(t *testing.T) {
	ctx := context.Background()
	ws, err := util.ParseUUID(testWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	inst := dbid.NewV7()
	suffix := util.UUIDToString(inst)
	member := dbfx.Insert(t, "user", testutil.Cols{
		"name": "routing member", "email": "routing-" + suffix + "@example.invalid",
	})
	dbfx.Insert(t, "member", testutil.Cols{
		"workspace_id": testWorkspaceID, "user_id": member, "role": "member",
	})
	runtime := dbfx.Runtime(t, "routing test")
	agent := dbfx.Agent(t, "routing test", runtime)
	_, err = testPool.Exec(ctx, `INSERT INTO channel_installation
		(id, workspace_id, agent_id, channel_type, config, installer_user_id)
		VALUES ($1, $2, $3, 'lweixin', $4, $5)`,
		inst, ws, agent, fmt.Sprintf(`{"app_id":"%s","silent_receive":true}`, suffix), testUserID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM lweixin_text_message WHERE conversation_id IN
			(SELECT id FROM lweixin_conversation WHERE installation_id = $1)`, inst)
		_, _ = testPool.Exec(ctx, `DELETE FROM lweixin_conversation WHERE installation_id = $1`, inst)
		_, _ = testPool.Exec(ctx, `DELETE FROM lweixin_routing_policy WHERE installation_id = $1`, inst)
		_, _ = testPool.Exec(ctx, `DELETE FROM channel_installation WHERE id = $1`, inst)
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
	convs, err := history.ListConversations(ctx, ws, inst, 50, 0)
	if err != nil || len(convs) != 1 {
		t.Fatalf("conversation: %+v %v", convs, err)
	}
	router := chi.NewRouter()
	router.Route("/api/workspaces/{id}", func(r chi.Router) {
		r.Use(middleware.RequireWorkspaceRoleFromURL(testHandler.Queries, "id", "owner", "admin"))
		r.Get("/lweixin/installations/{installationId}/routing", testHandler.GetLweixinRouting)
		r.Patch("/lweixin/installations/{installationId}/routing", testHandler.PatchLweixinRouting)
		r.Patch("/lweixin/installations/{installationId}/conversations/{conversationId}/route", testHandler.PatchLweixinConversationRoute)
		r.Get("/lweixin/installations/{installationId}/conversations", testHandler.ListLweixinConversations)
	})
	base := "/api/workspaces/" + testWorkspaceID + "/lweixin/installations/" + suffix
	call := func(method, path, body, user string, code int) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("X-User-ID", user)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != code {
			t.Fatalf("%s %s: %d %s (want %d)", method, path, rec.Code, rec.Body.String(), code)
		}
		if code != http.StatusOK {
			return nil
		}
		var result map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	defaults := call("GET", base+"/routing", "", testUserID, 200)
	if defaults["private_default"].(map[string]any)["mode"] != "silent" ||
		defaults["group_default"].(map[string]any)["mode"] != "silent" {
		t.Fatalf("unsafe defaults: %+v", defaults)
	}
	call("PATCH", base+"/routing", `{"private_default":{"mode":"agent","agent_id":"`+agent+`"}}`,
		member, 403)
	call("PATCH", base+"/routing", `{"private_default":{"mode":"agent","agent_id":"`+util.UUIDToString(dbid.NewV7())+`"}}`,
		testUserID, 409)
	call("PATCH", base+"/routing", `{"group_default":{"mode":"agent","agent_id":"`+agent+`"}}`,
		testUserID, 400)
	defaults = call("PATCH", base+"/routing", `{"private_default":{"mode":"agent","agent_id":"`+agent+`"}}`,
		testUserID, 200)
	if defaults["private_revision"] != float64(1) || defaults["group_revision"] != float64(0) {
		t.Fatalf("revisions: %+v", defaults)
	}
	defaults = call("PATCH", base+"/routing", `{"private_default":{"mode":"agent","agent_id":"`+agent+`"}}`,
		testUserID, 200)
	if defaults["private_revision"] != float64(1) {
		t.Fatalf("idempotent patch changed revision: %+v", defaults)
	}
	list := call("GET", base+"/conversations?limit=1&offset=0", "", testUserID, 200)
	got := list["conversations"].([]any)[0].(map[string]any)
	if got["effective_mode"] != "agent" || got["effective_agent_id"] != agent ||
		got["route_revision"] != float64(1) {
		t.Fatalf("inherited route: %+v", got)
	}
	route := base + "/conversations/" + convs[0].ID + "/route"
	got = call("PATCH", route, `{"mode":"silent"}`, testUserID, 200)
	if got["effective_mode"] != "silent" || got["route_revision"] != float64(2) {
		t.Fatalf("explicit silent route: %+v", got)
	}
	call("PATCH", route, `{"mode":"inherit","agent_id":"`+agent+`"}`, testUserID, 400)
	got = call("PATCH", route, `{"mode":"inherit"}`, testUserID, 200)
	if got["effective_mode"] != "agent" || got["route_revision"] != float64(3) {
		t.Fatalf("restored inheritance: %+v", got)
	}
	call("PATCH", base+"/conversations/"+util.UUIDToString(dbid.NewV7())+"/route", `{"mode":"silent"}`, testUserID, 404)
	if _, err := testPool.Exec(ctx, `UPDATE agent SET archived_at = now() WHERE id = $1`, agent); err != nil {
		t.Fatal(err)
	}
	call("PATCH", route, `{"mode":"agent","agent_id":"`+agent+`"}`, testUserID, 409)
	if _, err := testPool.Exec(ctx, `UPDATE channel_installation
		SET config = jsonb_set(config, '{app_id}', '"replacement"') WHERE id = $1`, inst); err != nil {
		t.Fatal(err)
	}
	defaults = call("GET", base+"/routing", "", testUserID, 200)
	if defaults["private_default"].(map[string]any)["mode"] != "silent" {
		t.Fatalf("replacement account inherited grant: %+v", defaults)
	}
	call("PATCH", route, `{"mode":"silent"}`, testUserID, 404)
}
