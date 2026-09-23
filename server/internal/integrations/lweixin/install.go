package lweixin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/integrations/channel/engine"
	"github.com/multica-ai/multica/server/internal/util/secretbox"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// InstallService is the LWEIXIN install backend, mirroring telegram's: the
// workspace admin points Multica at an already-running LWEIXIN server and
// pastes its API token. The service owns at-rest encryption of the token and
// the shared persist transaction; there is no live credential check because
// the bot session lives in the external LWEIXIN server, not here.

var (
	// ErrInstallationNotFound surfaces "no row matches in this workspace".
	ErrInstallationNotFound = errors.New("lweixin installation not found")
	// ErrBotOwnedByAnotherWorkspace: this LWEIXIN account is already
	// connected to a live owner in a DIFFERENT Multica workspace.
	ErrBotOwnedByAnotherWorkspace = errors.New("lweixin: this account is already connected to a different Multica workspace")
	// ErrBotOwnedBySameWorkspace: the account is already connected to a
	// different live agent in the SAME workspace.
	ErrBotOwnedBySameWorkspace = errors.New("lweixin: this account is already connected to another agent in this workspace")
	// ErrBotOwnedByArchivedAgent: the account's owning agent is archived.
	ErrBotOwnedByArchivedAgent = errors.New("lweixin: this account is connected to an archived agent in this workspace")
)

// installQueries is the slice of generated queries InstallService needs,
// interface-shaped so tests inject a fake.
type installQueries interface {
	WithTx(tx pgx.Tx) installQueries
	UpsertChannelInstallation(ctx context.Context, arg db.UpsertChannelInstallationParams) (db.ChannelInstallation, error)
	ReclaimDeadChannelInstallationByAppID(ctx context.Context, arg db.ReclaimDeadChannelInstallationByAppIDParams) (pgtype.UUID, error)
	GetChannelInstallationOwnerByAppID(ctx context.Context, arg db.GetChannelInstallationOwnerByAppIDParams) (db.GetChannelInstallationOwnerByAppIDRow, error)
	GetChannelInstallationByAppID(ctx context.Context, arg db.GetChannelInstallationByAppIDParams) (db.ChannelInstallation, error)
	ListChannelInstallationsByWorkspace(ctx context.Context, arg db.ListChannelInstallationsByWorkspaceParams) ([]db.ChannelInstallation, error)
	GetChannelInstallationInWorkspace(ctx context.Context, arg db.GetChannelInstallationInWorkspaceParams) (db.ChannelInstallation, error)
	SetChannelInstallationStatus(ctx context.Context, arg db.SetChannelInstallationStatusParams) error
}

type dbInstallQueries struct{ *db.Queries }

func (q dbInstallQueries) WithTx(tx pgx.Tx) installQueries {
	return dbInstallQueries{q.Queries.WithTx(tx)}
}

// InstallService owns the at-rest encryption of the API token and the install
// transaction. The box MUST be non-nil (plaintext storage is refused).
type InstallService struct {
	box    *secretbox.Box
	q      installQueries
	tx     engine.TxStarter
	logger *slog.Logger
}

// NewInstallService binds the service to queries, a tx starter, and an
// encryption box.
func NewInstallService(q *db.Queries, tx engine.TxStarter, box *secretbox.Box, logger *slog.Logger) (*InstallService, error) {
	if q == nil {
		return nil, errors.New("lweixin: InstallService requires queries")
	}
	if tx == nil {
		return nil, errors.New("lweixin: InstallService requires a tx starter")
	}
	if box == nil {
		return nil, errors.New("lweixin: InstallService requires a non-nil secretbox.Box")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &InstallService{box: box, q: dbInstallQueries{q}, tx: tx, logger: logger}, nil
}

// RegisterParams are the inputs for an install: the agent this LWEIXIN
// account answers for, who is installing, and the server coordinates.
type RegisterParams struct {
	WorkspaceID pgtype.UUID
	AgentID     pgtype.UUID
	InitiatorID pgtype.UUID
	// AppID is the bot account wxid as reported by the LWEIXIN server
	// (GET /api/status data.wxid); it fills the routing slot.
	AppID   string
	BaseURL string
	// APIToken is the LWEIXIN server's shared bearer token, sealed at rest.
	APIToken string
}

// Register persists the installation keyed by (workspace, agent) with the
// account id in the routing slot.
func (s *InstallService) Register(ctx context.Context, p RegisterParams) (db.ChannelInstallation, error) {
	sealed, err := s.box.Seal([]byte(p.APIToken))
	if err != nil {
		return db.ChannelInstallation{}, fmt.Errorf("encrypt lweixin api token: %w", err)
	}
	cfgJSON, err := json.Marshal(installConfig{
		AppID:             p.AppID,
		BaseURL:           p.BaseURL,
		APITokenEncrypted: base64.StdEncoding.EncodeToString(sealed),
		SilentReceive:     true,
	})
	if err != nil {
		return db.ChannelInstallation{}, fmt.Errorf("encode lweixin installation config: %w", err)
	}
	return s.persistInstall(ctx, installPersist{
		wsID:        p.WorkspaceID,
		agentID:     p.AgentID,
		installerID: p.InitiatorID,
		appIDKey:    p.AppID,
		configJSON:  cfgJSON,
	})
}

// installPersist carries the resolved fields persistInstall writes.
type installPersist struct {
	wsID        pgtype.UUID
	agentID     pgtype.UUID
	installerID pgtype.UUID
	appIDKey    string
	configJSON  []byte
}

const pgUniqueViolationLweixin = "23505"

// persistInstall upserts the installation keyed by (workspace_id, agent_id,
// channel_type): ONE LWEIXIN account per agent. A unique violation on the
// (channel_type, app_id) routing index means the account already serves a
// different live owner - refuse rather than steal.
func (s *InstallService) persistInstall(ctx context.Context, p installPersist) (db.ChannelInstallation, error) {
	tx, err := s.tx.Begin(ctx)
	if err != nil {
		return db.ChannelInstallation{}, fmt.Errorf("begin install tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	// Free the routing slot from any DEAD prior owner before the upsert.
	if _, err := qtx.ReclaimDeadChannelInstallationByAppID(ctx, db.ReclaimDeadChannelInstallationByAppIDParams{
		ChannelType: string(TypeLweixin),
		AppID:       p.appIDKey,
		WorkspaceID: p.wsID,
		AgentID:     p.agentID,
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return db.ChannelInstallation{}, fmt.Errorf("reclaim dead lweixin installation: %w", err)
	}
	// Reconnecting the same account must not switch a legacy installation
	// to silent before its routing policy is migrated by #8.
	previous, err := qtx.GetChannelInstallationByAppID(ctx, db.GetChannelInstallationByAppIDParams{
		ChannelType: string(TypeLweixin), AppID: p.appIDKey,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return db.ChannelInstallation{}, fmt.Errorf("load prior lweixin installation: %w", err)
	}
	if err == nil && previous.WorkspaceID == p.wsID && previous.AgentID == p.agentID {
		var old, next installConfig
		if err := json.Unmarshal(previous.Config, &old); err != nil {
			return db.ChannelInstallation{}, fmt.Errorf("decode prior lweixin config: %w", err)
		}
		if err := json.Unmarshal(p.configJSON, &next); err != nil {
			return db.ChannelInstallation{}, err
		}
		next.SilentReceive = old.SilentReceive
		p.configJSON, err = json.Marshal(next)
		if err != nil {
			return db.ChannelInstallation{}, err
		}
	}

	inst, err := qtx.UpsertChannelInstallation(ctx, db.UpsertChannelInstallationParams{
		WorkspaceID:     p.wsID,
		AgentID:         p.agentID,
		ChannelType:     string(TypeLweixin),
		Config:          p.configJSON,
		InstallerUserID: p.installerID,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolationLweixin {
			return db.ChannelInstallation{}, s.liveOwnerConflictErr(ctx, p.wsID, p.appIDKey)
		}
		return db.ChannelInstallation{}, fmt.Errorf("upsert lweixin installation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return db.ChannelInstallation{}, fmt.Errorf("commit lweixin install: %w", err)
	}
	return inst, nil
}

// liveOwnerConflictErr classifies who holds the routing slot so the handler
// renders an accurate message.
func (s *InstallService) liveOwnerConflictErr(ctx context.Context, requestingWorkspaceID pgtype.UUID, appID string) error {
	owner, err := s.q.GetChannelInstallationOwnerByAppID(ctx, db.GetChannelInstallationOwnerByAppIDParams{
		ChannelType: string(TypeLweixin),
		AppID:       appID,
	})
	if err != nil {
		return ErrBotOwnedByAnotherWorkspace
	}
	switch {
	case owner.WorkspaceID != requestingWorkspaceID:
		return ErrBotOwnedByAnotherWorkspace
	case owner.AgentArchivedAt.Valid:
		return ErrBotOwnedByArchivedAgent
	default:
		return ErrBotOwnedBySameWorkspace
	}
}

// ListByWorkspace returns every LWEIXIN installation in the workspace.
func (s *InstallService) ListByWorkspace(ctx context.Context, wsID pgtype.UUID) ([]db.ChannelInstallation, error) {
	return s.q.ListChannelInstallationsByWorkspace(ctx, db.ListChannelInstallationsByWorkspaceParams{
		WorkspaceID: wsID,
		ChannelType: string(TypeLweixin),
	})
}

// GetInWorkspace is the workspace-scoped lookup so a forged installation id
// from another workspace returns NotFound instead of leaking existence.
func (s *InstallService) GetInWorkspace(ctx context.Context, id, wsID pgtype.UUID) (db.ChannelInstallation, error) {
	inst, err := s.q.GetChannelInstallationInWorkspace(ctx, db.GetChannelInstallationInWorkspaceParams{
		ID:          id,
		WorkspaceID: wsID,
		ChannelType: string(TypeLweixin),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.ChannelInstallation{}, ErrInstallationNotFound
		}
		return db.ChannelInstallation{}, err
	}
	return inst, nil
}

// Revoke flips status to 'revoked'. The Supervisor stops the polling loop;
// outbound drops too. The row is preserved for audit.
func (s *InstallService) Revoke(ctx context.Context, id pgtype.UUID) error {
	return s.q.SetChannelInstallationStatus(ctx, db.SetChannelInstallationStatusParams{
		ID:     id,
		Status: "revoked",
	})
}
