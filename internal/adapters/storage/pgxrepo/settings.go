package pgxrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloud-lab-gateway/gateway/internal/adapters/storage/sqlcgen"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SettingsRepo persists scoped JSON settings in Postgres.
type SettingsRepo struct {
	q *sqlcgen.Queries
}

var _ ports.SettingsRepo = (*SettingsRepo)(nil)

func NewSettingsRepo(db *pgxpool.Pool) *SettingsRepo {
	return &SettingsRepo{q: sqlcgen.New(db)}
}

func (r *SettingsRepo) Resolve(ctx context.Context, key string, courseIDValue *shared.CourseID, templateID *shared.LabTemplateID) ([]byte, error) {
	row, err := r.q.ResolveSetting(ctx, sqlcgen.ResolveSettingParams{
		Key:           key,
		CourseID:      courseIDString(courseIDValue),
		LabTemplateID: labTemplateIDString(templateID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: resolve setting: %w", err)
	}
	return append([]byte(nil), row.Value...), nil
}

func (r *SettingsRepo) List(ctx context.Context, scope string, scopeID *string) ([]ports.Setting, error) {
	var scopeParam *string
	if scope != "" {
		scopeParam = &scope
	}
	rows, err := r.q.ListSettings(ctx, sqlcgen.ListSettingsParams{Scope: scopeParam, ScopeID: scopeID})
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: list settings: %w", err)
	}
	out := make([]ports.Setting, 0, len(rows))
	for _, row := range rows {
		out = append(out, settingFromRow(row))
	}
	return out, nil
}

func (r *SettingsRepo) Put(ctx context.Context, tx ports.Tx, s ports.Setting) error {
	q, err := r.queriesInTx(tx)
	if err != nil {
		return err
	}
	scopeID := ""
	if s.ScopeID != nil {
		scopeID = *s.ScopeID
	}
	if _, err := q.UpsertSetting(ctx, sqlcgen.UpsertSettingParams{
		Key:             s.Key,
		Scope:           s.Scope,
		ScopeID:         scopeID,
		Value:           s.Value,
		UpdatedByUserID: settingUserID(s.UpdatedByUser),
		UpdatedAt:       nullableTimeArg(s.UpdatedAt),
	}); err != nil {
		return fmt.Errorf("pgxrepo: put setting: %w", err)
	}
	return nil
}

func (r *SettingsRepo) queriesInTx(tx ports.Tx) (*sqlcgen.Queries, error) {
	pgxTx, err := txFromPort(tx)
	if err != nil {
		return nil, err
	}
	return r.q.WithTx(pgxTx), nil
}

func settingFromRow(row sqlcgen.Setting) ports.Setting {
	var updatedBy shared.UserID
	if row.UpdatedByUserID.Valid {
		updatedBy = shared.UserID(row.UpdatedByUserID.UUID)
	}
	var scopeID *string
	if row.ScopeID != "" {
		v := row.ScopeID
		scopeID = &v
	}
	return ports.Setting{
		Key:           row.Key,
		Value:         append([]byte(nil), row.Value...),
		Scope:         row.Scope,
		ScopeID:       scopeID,
		UpdatedByUser: updatedBy,
		UpdatedAt:     timeFromTimestamptz(row.UpdatedAt),
	}
}

func settingUserID(id shared.UserID) uuid.NullUUID {
	if id == (shared.UserID{}) {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: uuid.UUID(id), Valid: true}
}

func courseIDString(id *shared.CourseID) *string {
	if id == nil {
		return nil
	}
	v := id.String()
	return &v
}

func labTemplateIDString(id *shared.LabTemplateID) *string {
	if id == nil {
		return nil
	}
	v := id.String()
	return &v
}
