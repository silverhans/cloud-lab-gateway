package admin

import (
	"context"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/audit"
	"github.com/cloud-lab-gateway/gateway/internal/domain/identity"
	"github.com/cloud-lab-gateway/gateway/internal/domain/pool"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"go.uber.org/zap"
)

type QuotaCache interface {
	Read(ctx context.Context) (shared.QuotaSnapshot, time.Duration, error)
}

type Actor struct {
	UserID shared.UserID
	Role   identity.Role
}

type Deps struct {
	UoW        ports.UnitOfWork
	Pool       ports.PoolRepo
	Audit      ports.AuditRepo
	Settings   ports.SettingsRepo
	QuotaCache QuotaCache
	Clock      ports.Clock
	Logger     *zap.Logger
}

type PutSettingInput struct {
	Actor   Actor
	Key     string
	Value   []byte
	Scope   string
	ScopeID *string
}

func (d Deps) ListProjects(ctx context.Context, actor Actor, kiDomainID string, state *pool.State) ([]pool.Project, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	return d.Pool.ListByDomain(ctx, kiDomainID, state)
}

func (d Deps) QuarantineProject(ctx context.Context, actor Actor, id shared.ProjectID, reason string) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	now := d.now()
	return d.UoW.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		project, err := d.Pool.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if err := project.QuarantineNow(reason, now); err != nil {
			return err
		}
		return d.Pool.Save(ctx, tx, project)
	})
}

func (d Deps) ReleaseProject(ctx context.Context, actor Actor, id shared.ProjectID) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	now := d.now()
	return d.UoW.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		project, err := d.Pool.GetByID(ctx, id)
		if err != nil {
			return err
		}
		switch project.State {
		case pool.StateFree:
			return nil
		case pool.StateQuarantine:
			if err := project.ManualResolve(now); err != nil {
				return err
			}
			if err := project.MarkClean(now); err != nil {
				return err
			}
		case pool.StateCleaning:
			if err := project.MarkClean(now); err != nil {
				return err
			}
		default:
			return shared.ErrInvalidTransition{Entity: "project", From: string(project.State), To: string(pool.StateFree)}
		}
		return d.Pool.Save(ctx, tx, project)
	})
}

func (d Deps) GetQuota(ctx context.Context, actor Actor) (shared.QuotaSnapshot, error) {
	if err := requireAdmin(actor); err != nil {
		return shared.QuotaSnapshot{}, err
	}
	snap, _, err := d.QuotaCache.Read(ctx)
	return snap, err
}

func (d Deps) ListSettings(ctx context.Context, actor Actor, scope string, scopeID *string) ([]ports.Setting, error) {
	if err := requireTeacherOrAdmin(actor); err != nil {
		return nil, err
	}
	return d.Settings.List(ctx, scope, scopeID)
}

func (d Deps) PutSetting(ctx context.Context, in PutSettingInput) (ports.Setting, error) {
	if err := requireTeacherOrAdmin(in.Actor); err != nil {
		return ports.Setting{}, err
	}
	scopeID := normalizedScopeID(in.Scope, in.ScopeID)
	setting := ports.Setting{
		Key:           in.Key,
		Value:         append([]byte(nil), in.Value...),
		Scope:         in.Scope,
		ScopeID:       scopeID,
		UpdatedByUser: in.Actor.UserID,
		UpdatedAt:     d.now(),
	}
	if err := d.UoW.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		return d.Settings.Put(ctx, tx, setting)
	}); err != nil {
		return ports.Setting{}, err
	}
	return setting, nil
}

func (d Deps) QueryAudit(ctx context.Context, actor Actor, f ports.AuditFilter) ([]audit.AuditEvent, error) {
	if err := requireTeacherOrAdmin(actor); err != nil {
		return nil, err
	}
	return d.Audit.Query(ctx, f)
}

func requireAdmin(actor Actor) error {
	if actor.Role != identity.RoleAdmin {
		return shared.ErrForbidden
	}
	return nil
}

func requireTeacherOrAdmin(actor Actor) error {
	if actor.Role != identity.RoleAdmin && actor.Role != identity.RoleTeacher {
		return shared.ErrForbidden
	}
	return nil
}

func normalizedScopeID(scope string, scopeID *string) *string {
	if scope == "global" {
		return nil
	}
	if scopeID == nil || *scopeID == "" {
		return nil
	}
	v := *scopeID
	return &v
}

func (d Deps) now() time.Time {
	if d.Clock == nil {
		return time.Now().UTC()
	}
	return d.Clock.Now()
}
