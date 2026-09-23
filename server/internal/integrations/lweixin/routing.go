package lweixin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/util"
)

var (
	ErrRoutingLegacy         = errors.New("lweixin: legacy installation needs routing migration")
	ErrRouteInvalid          = errors.New("lweixin: invalid route")
	ErrRouteAgentUnavailable = errors.New("lweixin: agent unavailable in workspace")
)

type RouteChoice struct {
	Mode        string  `json:"mode"`
	AgentID     *string `json:"agent_id"`
	TriggerMode *string `json:"trigger_mode,omitempty"`
}

// Policies are storage-only until channel-task authorization can run without
// minting the runtime owner's member token. SilentHandler remains the ingress.
type RoutingPolicy struct {
	PrivateDefault  RouteChoice `json:"private_default"`
	GroupDefault    RouteChoice `json:"group_default"`
	PrivateRevision int64       `json:"private_revision"`
	GroupRevision   int64       `json:"group_revision"`
}

type ConversationRoute struct {
	Conversation
	RouteMode        string  `json:"route_mode"`
	RouteAgentID     *string `json:"route_agent_id"`
	EffectiveMode    string  `json:"effective_mode"`
	EffectiveAgentID *string `json:"effective_agent_id"`
	RouteRevision    int64   `json:"route_revision"`
	TriggerMode      string  `json:"trigger_mode,omitempty"`
	TriggerReason    string  `json:"trigger_reason,omitempty"`
}

func setGroupTrigger(c *ConversationRoute, trigger string) {
	if c.ChatType != "group" {
		return
	}
	c.TriggerMode = trigger
	if trigger == "mention" {
		c.TriggerReason = "mention_metadata_unavailable"
	} else {
		c.TriggerReason = "execution_isolation_unavailable"
	}
}

func uuidPointer(id pgtype.UUID) *string {
	if !id.Valid {
		return nil
	}
	s := util.UUIDToString(id)
	return &s
}

func routeAgent(choice RouteChoice) (pgtype.UUID, error) {
	if choice.Mode == "silent" || choice.Mode == "inherit" {
		if choice.AgentID != nil {
			return pgtype.UUID{}, ErrRouteInvalid
		}
		return pgtype.UUID{}, nil
	}
	if choice.Mode != "agent" || choice.AgentID == nil {
		return pgtype.UUID{}, ErrRouteInvalid
	}
	id, err := util.ParseUUID(*choice.AgentID)
	if err != nil || !id.Valid {
		return pgtype.UUID{}, ErrRouteInvalid
	}
	return id, nil
}

func (h *History) routingInstallation(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, ws, inst pgtype.UUID, lock bool) (string, error) {
	sql := `SELECT config FROM channel_installation WHERE id = $1
		AND workspace_id = $2 AND channel_type = 'lweixin' AND status = 'active'`
	if lock {
		sql += " FOR UPDATE"
	}
	var raw []byte
	if err := q.QueryRow(ctx, sql, inst, ws).Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrHistoryNotFound
		}
		return "", err
	}
	var cfg installConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", err
	}
	if !cfg.SilentReceive || cfg.AppID == "" {
		return "", ErrRoutingLegacy
	}
	return cfg.AppID, nil
}

func (h *History) getRouting(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, inst pgtype.UUID, account string) (RoutingPolicy, error) {
	p := RoutingPolicy{
		PrivateDefault: RouteChoice{Mode: "silent"},
		GroupDefault:   RouteChoice{Mode: "silent"},
	}
	var private, group pgtype.UUID
	err := q.QueryRow(ctx, `SELECT private_mode, private_agent_id, private_revision,
		group_mode, group_agent_id, group_revision FROM lweixin_routing_policy
		WHERE installation_id = $1 AND account_id = $2`, inst, account).Scan(
		&p.PrivateDefault.Mode, &private, &p.PrivateRevision,
		&p.GroupDefault.Mode, &group, &p.GroupRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, nil
	}
	p.PrivateDefault.AgentID, p.GroupDefault.AgentID = uuidPointer(private), uuidPointer(group)
	return p, err
}

func (h *History) GetRouting(ctx context.Context, ws, inst pgtype.UUID) (RoutingPolicy, error) {
	account, err := h.routingInstallation(ctx, h.pool, ws, inst, false)
	if err != nil {
		return RoutingPolicy{}, err
	}
	return h.getRouting(ctx, h.pool, inst, account)
}

func validateRouteAgent(ctx context.Context, tx pgx.Tx, ws, agent pgtype.UUID) error {
	if !agent.Valid {
		return nil
	}
	var id pgtype.UUID
	err := tx.QueryRow(ctx, `SELECT a.id FROM agent a
		JOIN agent_runtime rt ON rt.id = a.runtime_id AND rt.workspace_id = a.workspace_id
		WHERE a.id = $1 AND a.workspace_id = $2 AND a.kind = 'user'
		  AND a.archived_at IS NULL AND rt.status = 'online'
		FOR SHARE OF a, rt`, agent, ws).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRouteAgentUnavailable
	}
	return err
}

func (h *History) PatchRouting(ctx context.Context, ws, inst pgtype.UUID, private, group *RouteChoice) (RoutingPolicy, error) {
	if private == nil && group == nil {
		return RoutingPolicy{}, ErrRouteInvalid
	}
	var privateID pgtype.UUID
	var err error
	if private != nil {
		if private.Mode == "inherit" || private.TriggerMode != nil {
			return RoutingPolicy{}, ErrRouteInvalid
		}
		privateID, err = routeAgent(*private)
		if err != nil {
			return RoutingPolicy{}, err
		}
	}
	var groupID pgtype.UUID
	if group != nil {
		if group.Mode == "inherit" || group.TriggerMode != nil {
			return RoutingPolicy{}, ErrRouteInvalid
		}
		groupID, err = routeAgent(*group)
		if err != nil {
			return RoutingPolicy{}, err
		}
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return RoutingPolicy{}, err
	}
	defer tx.Rollback(ctx)
	account, err := h.routingInstallation(ctx, tx, ws, inst, true)
	if err != nil {
		return RoutingPolicy{}, err
	}
	if err := validateRouteAgent(ctx, tx, ws, privateID); err != nil {
		return RoutingPolicy{}, err
	}
	if err := validateRouteAgent(ctx, tx, ws, groupID); err != nil {
		return RoutingPolicy{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO lweixin_routing_policy (workspace_id, installation_id, account_id)
		VALUES ($1, $2, $3) ON CONFLICT (installation_id, account_id) DO NOTHING`, ws, inst, account); err != nil {
		return RoutingPolicy{}, err
	}
	before, err := h.getRouting(ctx, tx, inst, account)
	if err != nil {
		return RoutingPolicy{}, err
	}
	if private != nil && (before.PrivateDefault.Mode != private.Mode ||
		(before.PrivateDefault.AgentID == nil) != (private.AgentID == nil) ||
		(before.PrivateDefault.AgentID != nil && private.AgentID != nil &&
			*before.PrivateDefault.AgentID != util.UUIDToString(privateID))) {
		_, err = tx.Exec(ctx, `UPDATE lweixin_routing_policy
			SET private_revision = private_revision + 1,
			private_mode = $3, private_agent_id = $4
			WHERE installation_id = $1 AND account_id = $2`,
			inst, account, private.Mode, privateID)
		if err != nil {
			return RoutingPolicy{}, err
		}
		// A changed inherited default invalidates tasks for every affected chat.
		_, err = tx.Exec(ctx, `UPDATE lweixin_conversation SET route_revision = route_revision + 1
			WHERE installation_id = $1 AND account_id = $2
			  AND chat_type = 'p2p' AND route_mode = 'inherit'`, inst, account)
		if err != nil {
			return RoutingPolicy{}, err
		}
	}
	if group != nil && (before.GroupDefault.Mode != group.Mode ||
		(before.GroupDefault.AgentID == nil) != (group.AgentID == nil) ||
		(before.GroupDefault.AgentID != nil && group.AgentID != nil &&
			*before.GroupDefault.AgentID != util.UUIDToString(groupID))) {
		_, err = tx.Exec(ctx, `UPDATE lweixin_routing_policy
			SET group_revision = group_revision + 1,
			group_mode = $3, group_agent_id = $4
			WHERE installation_id = $1 AND account_id = $2`,
			inst, account, group.Mode, groupID)
		if err != nil {
			return RoutingPolicy{}, err
		}
		_, err = tx.Exec(ctx, `UPDATE lweixin_conversation SET route_revision = route_revision + 1
			WHERE installation_id = $1 AND account_id = $2
			  AND chat_type = 'group' AND route_mode = 'inherit'`, inst, account)
		if err != nil {
			return RoutingPolicy{}, err
		}
	}
	result, err := h.getRouting(ctx, tx, inst, account)
	if err != nil {
		return RoutingPolicy{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RoutingPolicy{}, err
	}
	return result, nil
}

func (h *History) conversationRoute(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, ws, inst, conv pgtype.UUID, account string) (ConversationRoute, error) {
	var c ConversationRoute
	var override, effective pgtype.UUID
	var trigger string
	err := q.QueryRow(ctx, `SELECT c.id::text, c.account_id, c.chat_type, c.chat_id,
		c.last_message_at, c.route_mode, c.route_agent_id,
		CASE WHEN c.route_mode = 'inherit' THEN
			CASE WHEN c.chat_type = 'group' THEN COALESCE(p.group_mode, 'silent')
				ELSE COALESCE(p.private_mode, 'silent') END
			ELSE c.route_mode END,
		CASE WHEN c.route_mode = 'inherit' THEN
			CASE WHEN c.chat_type = 'group' THEN p.group_agent_id
				ELSE p.private_agent_id END
			ELSE c.route_agent_id END, c.route_revision, c.trigger_mode
		FROM lweixin_conversation c
		LEFT JOIN lweixin_routing_policy p ON p.installation_id = c.installation_id
			AND p.account_id = c.account_id
		WHERE c.id = $1 AND c.workspace_id = $2 AND c.installation_id = $3
		  AND c.account_id = $4`, conv, ws, inst, account).Scan(
		&c.ID, &c.AccountID, &c.ChatType, &c.ChatID, &c.LastMessageAt,
		&c.RouteMode, &override, &c.EffectiveMode, &effective, &c.RouteRevision, &trigger)
	if errors.Is(err, pgx.ErrNoRows) {
		return ConversationRoute{}, ErrHistoryNotFound
	}
	c.RouteAgentID, c.EffectiveAgentID = uuidPointer(override), uuidPointer(effective)
	setGroupTrigger(&c, trigger)
	return c, err
}

func (h *History) PatchConversationRoute(ctx context.Context, ws, inst, conv pgtype.UUID, choice RouteChoice) (ConversationRoute, error) {
	agent, err := routeAgent(choice)
	if err != nil {
		return ConversationRoute{}, err
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return ConversationRoute{}, err
	}
	defer tx.Rollback(ctx)
	account, err := h.routingInstallation(ctx, tx, ws, inst, true)
	if err != nil {
		return ConversationRoute{}, err
	}
	if err := validateRouteAgent(ctx, tx, ws, agent); err != nil {
		return ConversationRoute{}, err
	}
	if choice.TriggerMode != nil && *choice.TriggerMode != "mention" && *choice.TriggerMode != "all" {
		return ConversationRoute{}, ErrRouteInvalid
	}
	if choice.TriggerMode != nil {
		var chatType string
		err := tx.QueryRow(ctx, `SELECT chat_type FROM lweixin_conversation
			WHERE id = $1 AND workspace_id = $2 AND installation_id = $3 AND account_id = $4`,
			conv, ws, inst, account).Scan(&chatType)
		if errors.Is(err, pgx.ErrNoRows) {
			return ConversationRoute{}, ErrHistoryNotFound
		}
		if err != nil {
			return ConversationRoute{}, err
		}
		if chatType != "group" {
			return ConversationRoute{}, ErrRouteInvalid
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE lweixin_conversation
		SET route_revision = route_revision + CASE WHEN
			(route_mode, route_agent_id, trigger_mode)
				IS DISTINCT FROM ($5::text, $6::uuid, COALESCE($7::text, trigger_mode))
			THEN 1 ELSE 0 END, route_mode = $5, route_agent_id = $6,
			trigger_mode = COALESCE($7::text, trigger_mode)
		WHERE id = $1 AND workspace_id = $2 AND installation_id = $3
		  AND account_id = $4`,
		conv, ws, inst, account, choice.Mode, agent, choice.TriggerMode)
	if err != nil {
		return ConversationRoute{}, fmt.Errorf("update lweixin route: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ConversationRoute{}, ErrHistoryNotFound
	}
	result, err := h.conversationRoute(ctx, tx, ws, inst, conv, account)
	if err != nil {
		return ConversationRoute{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ConversationRoute{}, err
	}
	return result, nil
}
