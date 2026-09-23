package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/integrations/lweixin"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func lweixinPage(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	limit, offset := 50, 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 100")
			return 0, 0, false
		}
		limit = n
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 1000000 {
			writeError(w, http.StatusBadRequest, "invalid offset")
			return 0, 0, false
		}
		offset = n
	}
	return limit, offset, true
}

func (h *Handler) lweixinHistoryScope(w http.ResponseWriter, r *http.Request) (pgtype.UUID, pgtype.UUID, bool) {
	if h.LweixinHistory == nil {
		writeFeatureDisabled(w, "lweixin_not_configured", "lweixin integration not configured")
		return pgtype.UUID{}, pgtype.UUID{}, false
	}
	ws, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workspace id")
	if !ok {
		return pgtype.UUID{}, pgtype.UUID{}, false
	}
	inst, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "installationId"), "installation id")
	return ws, inst, ok
}

func (h *Handler) ListLweixinConversations(w http.ResponseWriter, r *http.Request) {
	ws, inst, ok := h.lweixinHistoryScope(w, r)
	if !ok {
		return
	}
	limit, offset, ok := lweixinPage(w, r)
	if !ok {
		return
	}
	rows, err := h.LweixinHistory.ListConversations(r.Context(), ws, inst, limit, offset)
	if errors.Is(err, lweixin.ErrHistoryNotFound) {
		writeError(w, http.StatusNotFound, "lweixin installation not found")
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list lweixin conversations")
	} else {
		writeJSON(w, http.StatusOK, map[string]any{"conversations": rows})
	}
}

func (h *Handler) ListLweixinMessages(w http.ResponseWriter, r *http.Request) {
	ws, inst, ok := h.lweixinHistoryScope(w, r)
	if !ok {
		return
	}
	conv, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "conversationId"), "conversation id")
	if !ok {
		return
	}
	limit, offset, ok := lweixinPage(w, r)
	if !ok {
		return
	}
	rows, err := h.LweixinHistory.ListMessages(r.Context(), ws, inst, conv, limit, offset)
	if errors.Is(err, lweixin.ErrHistoryNotFound) {
		writeError(w, http.StatusNotFound, "lweixin conversation not found")
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list lweixin messages")
	} else {
		writeJSON(w, http.StatusOK, map[string]any{"messages": rows})
	}
}

func lweixinRouteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, lweixin.ErrHistoryNotFound):
		writeError(w, http.StatusNotFound, "lweixin installation or conversation not found")
	case errors.Is(err, lweixin.ErrRoutingLegacy), errors.Is(err, lweixin.ErrRouteAgentUnavailable):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, lweixin.ErrRouteInvalid):
		writeError(w, http.StatusBadRequest, "invalid lweixin route")
	default:
		writeError(w, http.StatusInternalServerError, "failed to update lweixin route")
	}
}

func (h *Handler) GetLweixinRouting(w http.ResponseWriter, r *http.Request) {
	ws, inst, ok := h.lweixinHistoryScope(w, r)
	if !ok {
		return
	}
	policy, err := h.LweixinHistory.GetRouting(r.Context(), ws, inst)
	if err != nil {
		lweixinRouteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, policy)
}

func (h *Handler) PatchLweixinRouting(w http.ResponseWriter, r *http.Request) {
	ws, inst, ok := h.lweixinHistoryScope(w, r)
	if !ok {
		return
	}
	var body struct {
		PrivateDefault *lweixin.RouteChoice `json:"private_default"`
		GroupDefault   *lweixin.RouteChoice `json:"group_default"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid routing policy")
		return
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid routing policy")
		return
	}
	policy, err := h.LweixinHistory.PatchRouting(r.Context(), ws, inst, body.PrivateDefault, body.GroupDefault)
	if err != nil {
		lweixinRouteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, policy)
}

func (h *Handler) PatchLweixinConversationRoute(w http.ResponseWriter, r *http.Request) {
	ws, inst, ok := h.lweixinHistoryScope(w, r)
	if !ok {
		return
	}
	conv, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "conversationId"), "conversation id")
	if !ok {
		return
	}
	var choice lweixin.RouteChoice
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&choice); err != nil {
		writeError(w, http.StatusBadRequest, "invalid conversation route")
		return
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid conversation route")
		return
	}
	result, err := h.LweixinHistory.PatchConversationRoute(r.Context(), ws, inst, conv, choice)
	if err != nil {
		lweixinRouteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// LweixinInstallationResponse is the wire shape for a LWEIXIN installation
// row. The encrypted API token in config is INTENTIONALLY absent: server-internal.
type LweixinInstallationResponse struct {
	ID              string `json:"id"`
	WorkspaceID     string `json:"workspace_id"`
	AgentID         string `json:"agent_id"`
	AppID           string `json:"app_id"`
	BaseURL         string `json:"base_url"`
	InstallerUserID string `json:"installer_user_id"`
	Status          string `json:"status"`
	InstalledAt     string `json:"installed_at"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

func lweixinInstallationToResponse(row db.ChannelInstallation) LweixinInstallationResponse {
	info := lweixin.DecodePublicConfig(row.Config)
	return LweixinInstallationResponse{
		ID:              uuidToString(row.ID),
		WorkspaceID:     uuidToString(row.WorkspaceID),
		AgentID:         uuidToString(row.AgentID),
		AppID:           info.AppID,
		BaseURL:         info.BaseURL,
		InstallerUserID: uuidToString(row.InstallerUserID),
		Status:          row.Status,
		InstalledAt:     row.InstalledAt.Time.UTC().Format(time.RFC3339),
		CreatedAt:       row.CreatedAt.Time.UTC().Format(time.RFC3339),
		UpdatedAt:       row.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}
}

// ListLweixinInstallations (GET /api/workspaces/{id}/lweixin/installations)
// is member-visible so the Integrations tab renders for non-admins.
func (h *Handler) ListLweixinInstallations(w http.ResponseWriter, r *http.Request) {
	if h.LweixinInstall == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"installations":     []LweixinInstallationResponse{},
			"configured":        false,
			"install_supported": false,
		})
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workspace id")
	if !ok {
		return
	}
	rows, err := h.LweixinInstall.ListByWorkspace(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list lweixin installations")
		return
	}
	out := make([]LweixinInstallationResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, lweixinInstallationToResponse(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"installations":     out,
		"configured":        true,
		"install_supported": true,
	})
}

// RegisterLweixinRequest is the body for an install: coordinates of an
// already-running LWEIXIN server.
type RegisterLweixinRequest struct {
	BaseURL  string `json:"base_url"`
	APIToken string `json:"api_token"`
	// AppID is the bot account wxid reported by GET /api/status. Optional:
	// the server is probed for it when omitted.
	AppID string `json:"app_id"`
}

// RegisterLweixinBot (POST /api/workspaces/{id}/lweixin/install?agent_id=...)
// stores a LWEIXIN server binding for an agent. Admin-only at the router.
// When app_id is omitted the handler probes base_url/api/status once with
// the token to learn the bot wxid, so a 200 here also proves reachability.
// Nothing is persisted when the probe fails.
func (h *Handler) RegisterLweixinBot(w http.ResponseWriter, r *http.Request) {
	if h.LweixinInstall == nil {
		writeFeatureDisabled(w, "lweixin_not_configured", "lweixin integration not enabled")
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workspace id")
	if !ok {
		return
	}
	agentIDStr := strings.TrimSpace(r.URL.Query().Get("agent_id"))
	if agentIDStr == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}
	agentUUID, ok := parseUUIDOrBadRequest(w, agentIDStr, "agent_id")
	if !ok {
		return
	}
	if _, err := h.Queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{
		ID:          agentUUID,
		WorkspaceID: wsUUID,
	}); err != nil {
		writeError(w, http.StatusNotFound, "agent not found in this workspace")
		return
	}
	initiatorUUID, ok := parseUUIDOrBadRequest(w, userID, "user id")
	if !ok {
		return
	}
	var body RegisterLweixinRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	body.BaseURL = strings.TrimRight(strings.TrimSpace(body.BaseURL), "/")
	if body.BaseURL == "" {
		writeError(w, http.StatusBadRequest, "base_url is required")
		return
	}
	appID := strings.TrimSpace(body.AppID)
	if appID == "" {
		st, err := lweixin.NewProbe(body.BaseURL, body.APIToken).Status(r.Context())
		if err != nil || st.Wxid == "" {
			writeError(w, http.StatusBadGateway, "could not read the account wxid from the LWEIXIN server; pass app_id explicitly. Nothing was saved")
			return
		}
		appID = st.Wxid
	}
	row, err := h.LweixinInstall.Register(r.Context(), lweixin.RegisterParams{
		WorkspaceID: wsUUID,
		AgentID:     agentUUID,
		InitiatorID: initiatorUUID,
		AppID:       appID,
		BaseURL:     body.BaseURL,
		APIToken:    body.APIToken,
	})
	if err != nil {
		switch {
		case errors.Is(err, lweixin.ErrBotOwnedBySameWorkspace):
			writeError(w, http.StatusConflict, "this LWEIXIN account is already connected to another agent in this workspace; disconnect it there first")
		case errors.Is(err, lweixin.ErrBotOwnedByArchivedAgent):
			writeError(w, http.StatusConflict, "this LWEIXIN account is connected to an archived agent in this workspace; restore that agent or disconnect it first")
		case errors.Is(err, lweixin.ErrBotOwnedByAnotherWorkspace):
			writeError(w, http.StatusConflict, "this LWEIXIN account is already connected to a different Multica workspace; disconnect it there first")
		default:
			writeError(w, http.StatusInternalServerError, "could not save this LWEIXIN connection; the token was not saved")
		}
		return
	}
	h.publish(protocol.EventLweixinInstallationCreated, uuidToString(row.WorkspaceID), "user", userID, map[string]any{
		"id": uuidToString(row.ID),
	})
	writeJSON(w, http.StatusOK, lweixinInstallationToResponse(row))
}

// RevokeLweixinInstallation (DELETE .../lweixin/installations/{installationId})
// flips status to revoked. Admin-only at the router. A re-install flips it
// back to active.
func (h *Handler) RevokeLweixinInstallation(w http.ResponseWriter, r *http.Request) {
	if h.LweixinInstall == nil {
		writeFeatureDisabled(w, "lweixin_not_configured", "lweixin integration not configured")
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workspace id")
	if !ok {
		return
	}
	instUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "installationId"), "installation id")
	if !ok {
		return
	}
	if _, err := h.LweixinInstall.GetInWorkspace(r.Context(), instUUID, wsUUID); err != nil {
		if errors.Is(err, lweixin.ErrInstallationNotFound) {
			writeError(w, http.StatusNotFound, "lweixin installation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load installation")
		return
	}
	if err := h.LweixinInstall.Revoke(r.Context(), instUUID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke installation")
		return
	}
	h.publish(protocol.EventLweixinInstallationRevoked, uuidToString(wsUUID), "user", userID, map[string]any{
		"id": uuidToString(instUUID),
	})
	w.WriteHeader(http.StatusNoContent)
}

// RedeemLweixinBindingTokenRequest carries the raw token from the
// link-account prompt.
type RedeemLweixinBindingTokenRequest struct {
	Token string `json:"token"`
}

// RedeemLweixinBindingTokenResponse echoes the bound identifiers.
type RedeemLweixinBindingTokenResponse struct {
	WorkspaceID    string `json:"workspace_id"`
	InstallationID string `json:"installation_id"`
	LweixinUserID  string `json:"lweixin_user_id"`
}

// RedeemLweixinBindingToken (POST /api/lweixin/binding/redeem) binds the
// wxid carried by the token to the logged-in Multica user. Identity comes
// from the session, never from the token.
func (h *Handler) RedeemLweixinBindingToken(w http.ResponseWriter, r *http.Request) {
	if h.LweixinBindingTokens == nil {
		writeFeatureDisabled(w, "lweixin_not_configured", "lweixin integration not configured")
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var req RedeemLweixinBindingTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	userUUID, ok := parseUUIDOrBadRequest(w, userID, "user id")
	if !ok {
		return
	}
	redeemed, err := h.LweixinBindingTokens.RedeemAndBind(r.Context(), req.Token, userUUID)
	if err != nil {
		switch {
		case errors.Is(err, lweixin.ErrBindingTokenInvalid):
			writeError(w, http.StatusGone, "binding token invalid or expired")
		case errors.Is(err, lweixin.ErrBindingAlreadyAssigned):
			writeError(w, http.StatusConflict, "this LWEIXIN account is already bound to a different Multica user")
		case errors.Is(err, lweixin.ErrBindingNotWorkspaceMember):
			writeError(w, http.StatusForbidden, "binding refused (are you a workspace member?)")
		default:
			writeError(w, http.StatusInternalServerError, "failed to redeem token")
		}
		return
	}
	writeJSON(w, http.StatusOK, RedeemLweixinBindingTokenResponse{
		WorkspaceID:    uuidToString(redeemed.WorkspaceID),
		InstallationID: uuidToString(redeemed.InstallationID),
		LweixinUserID:  redeemed.LweixinUserID,
	})
}
