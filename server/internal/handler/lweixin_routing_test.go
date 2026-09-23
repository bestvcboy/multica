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
	msg.MessageID = "group-one"
	msg.Source.ChatType, msg.Source.ChatID, msg.Source.SenderID = channel.ChatTypeGroup, "room", "member"
	if err := history.SilentHandler(inst)(ctx, msg); err != nil {
		t.Fatal(err)
	}
	msg.MessageID, msg.Source.ChatID = "group-two", "room-two"
	if err := history.SilentHandler(inst)(ctx, msg); err != nil {
		t.Fatal(err)
	}
	convs, err := history.ListConversations(ctx, ws, inst, 50, 0)
	if err != nil || len(convs) != 3 {
		t.Fatalf("conversation: %+v %v", convs, err)
	}
	var privateID, groupID, otherGroupID string
	for _, conv := range convs {
		switch conv.ChatID {
		case "friend":
			privateID = conv.ID
		case "room":
			groupID = conv.ID
		case "room-two":
			otherGroupID = conv.ID
		}
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
	call("PATCH", base+"/routing", `{"group_default":{"mode":"agent","agent_id":"`+util.UUIDToString(dbid.NewV7())+`"}}`,
		testUserID, 409)
	defaults = call("PATCH", base+"/routing", `{"group_default":{"mode":"agent","agent_id":"`+agent+`"}}`,
		testUserID, 200)
	if defaults["group_revision"] != float64(1) || defaults["private_revision"] != float64(0) {
		t.Fatalf("group default affected private: %+v", defaults)
	}
	defaults = call("PATCH", base+"/routing", `{"private_default":{"mode":"agent","agent_id":"`+agent+`"}}`,
		testUserID, 200)
	if defaults["private_revision"] != float64(1) || defaults["group_revision"] != float64(1) {
		t.Fatalf("revisions: %+v", defaults)
	}
	defaults = call("PATCH", base+"/routing", `{"private_default":{"mode":"agent","agent_id":"`+agent+`"}}`,
		testUserID, 200)
	if defaults["private_revision"] != float64(1) {
		t.Fatalf("idempotent patch changed revision: %+v", defaults)
	}
	list := call("GET", base+"/conversations?limit=50&offset=0", "", testUserID, 200)
	var got, group, otherGroup map[string]any
	for _, row := range list["conversations"].([]any) {
		item := row.(map[string]any)
		switch item["id"] {
		case groupID:
			group = item
		case otherGroupID:
			otherGroup = item
		case privateID:
			got = item
		}
	}
	if got["effective_mode"] != "agent" || got["effective_agent_id"] != agent ||
		got["route_revision"] != float64(1) {
		t.Fatalf("inherited route: %+v", got)
	}
	if group["effective_mode"] != "agent" || group["trigger_mode"] != "mention" ||
		group["trigger_reason"] != "mention_metadata_unavailable" || group["route_revision"] != float64(1) {
		t.Fatalf("inherited group: %+v", group)
	}
	if otherGroup["effective_mode"] != "agent" || otherGroup["trigger_mode"] != "mention" ||
		otherGroup["route_revision"] != float64(1) {
		t.Fatalf("second group inherited separately: %+v", otherGroup)
	}
	groupRoute := base + "/conversations/" + groupID + "/route"
	call("PATCH", groupRoute, `{"mode":"inherit","trigger_mode":"invalid"}`, testUserID, 400)
	call("PATCH", groupRoute, `{"mode":"agent","agent_id":"`+util.UUIDToString(dbid.NewV7())+`"}`, testUserID, 409)
	group = call("PATCH", groupRoute, `{"mode":"inherit","trigger_mode":"all"}`, testUserID, 200)
	if group["trigger_mode"] != "all" || group["trigger_reason"] != "execution_isolation_unavailable" ||
		group["route_revision"] != float64(2) {
		t.Fatalf("group all: %+v", group)
	}
	group = call("PATCH", groupRoute, `{"mode":"inherit","trigger_mode":"all"}`, testUserID, 200)
	if group["route_revision"] != float64(2) {
		t.Fatalf("idempotent group patch: %+v", group)
	}
	group = call("PATCH", groupRoute, `{"mode":"silent"}`, testUserID, 200)
	if group["effective_mode"] != "silent" || group["route_revision"] != float64(3) {
		t.Fatalf("group override: %+v", group)
	}
	defaults = call("PATCH", base+"/routing", `{"group_default":{"mode":"silent"}}`, testUserID, 200)
	if defaults["group_revision"] != float64(2) || defaults["private_revision"] != float64(1) {
		t.Fatalf("independent group revision: %+v", defaults)
	}
	list = call("GET", base+"/conversations?limit=50", "", testUserID, 200)
	for _, row := range list["conversations"].([]any) {
		item := row.(map[string]any)
		if item["id"] == otherGroupID && (item["effective_mode"] != "silent" || item["route_revision"] != float64(2)) {
			t.Fatalf("second group default did not change independently: %+v", item)
		}
	}
	group = call("PATCH", groupRoute, `{"mode":"agent","agent_id":"`+agent+`"}`, testUserID, 200)
	if group["effective_mode"] != "agent" || group["effective_agent_id"] != agent ||
		group["route_revision"] != float64(4) {
		t.Fatalf("explicit group agent: %+v", group)
	}
	group = call("PATCH", groupRoute, `{"mode":"inherit","trigger_mode":"mention"}`, testUserID, 200)
	if group["effective_mode"] != "silent" || group["route_revision"] != float64(5) {
		t.Fatalf("restored group inheritance: %+v", group)
	}
	defaults = call("PATCH", base+"/routing", `{"group_default":{"mode":"agent","agent_id":"`+agent+`"}}`,
		testUserID, 200)
	if defaults["group_revision"] != float64(3) || defaults["private_revision"] != float64(1) {
		t.Fatalf("group default after override: %+v", defaults)
	}
	list = call("GET", base+"/conversations?limit=50", "", testUserID, 200)
	for _, row := range list["conversations"].([]any) {
		item := row.(map[string]any)
		if item["id"] == groupID && (item["effective_mode"] != "agent" || item["route_revision"] != float64(6)) {
			t.Fatalf("inherited group default not fenced: %+v", item)
		}
		if item["id"] == otherGroupID && (item["effective_mode"] != "agent" || item["route_revision"] != float64(3)) {
			t.Fatalf("second group default not fenced: %+v", item)
		}
	}
	route := base + "/conversations/" + privateID + "/route"
	call("PATCH", route, `{"mode":"inherit","trigger_mode":"all"}`, testUserID, 400)
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
	call("PATCH", groupRoute, `{"mode":"agent","agent_id":"`+agent+`"}`, testUserID, 409)
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
