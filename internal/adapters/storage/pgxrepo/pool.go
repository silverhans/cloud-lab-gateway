package pgxrepo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/adapters/storage/sqlcgen"
	"github.com/cloud-lab-gateway/gateway/internal/domain/pool"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolRepo persists pool.Project aggregates in Postgres.
type PoolRepo struct {
	db *pgxpool.Pool
	q  *sqlcgen.Queries
}

var _ ports.PoolRepo = (*PoolRepo)(nil)

// NewPoolRepo creates a PoolRepo backed by a pgx pool.
func NewPoolRepo(db *pgxpool.Pool) *PoolRepo {
	return &PoolRepo{db: db, q: sqlcgen.New(db)}
}

// AllocateOneFree atomically reserves one free project with FOR UPDATE SKIP LOCKED.
func (r *PoolRepo) AllocateOneFree(ctx context.Context, tx ports.Tx, kiDomainID string, labIDValue shared.LabInstanceID) (*pool.Project, error) {
	q, err := r.queriesInTx(tx)
	if err != nil {
		return nil, err
	}

	row, err := q.AllocateOneFreeProject(ctx, sqlcgen.AllocateOneFreeProjectParams{
		KiDomainID:       kiDomainID,
		AllocatedToLabID: uuid.NullUUID{UUID: labID(labIDValue), Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrPoolEmpty
	}
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: allocate free project: %w", err)
	}
	return projectFromRow(row), nil
}

// Save persists the project state and flushes pending domain events to outbox and audit.
func (r *PoolRepo) Save(ctx context.Context, tx ports.Tx, p *pool.Project) error {
	if p == nil {
		return shared.ErrInvalidInput
	}
	q, err := r.queriesInTx(tx)
	if err != nil {
		return err
	}
	cleanupFailures, err := cleanupFailuresInt32(p.CleanupFailures)
	if err != nil {
		return err
	}

	_, err = q.UpdateProject(ctx, sqlcgen.UpdateProjectParams{
		ID:                projectID(p.ID),
		State:             string(p.State),
		AllocatedToLabID:  nullableAllocatedLabIDForState(p),
		CleanupFailures:   cleanupFailures,
		LastStateChangeAt: timestamptz(p.LastStateChangeAt),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return shared.ErrIdempotencyClash
	}
	if err != nil {
		return fmt.Errorf("pgxrepo: save project: %w", err)
	}

	for _, ev := range p.PullEvents() {
		if err := r.appendProjectEvent(ctx, q, ev, p.LastStateChangeAt); err != nil {
			return err
		}
	}
	return nil
}

// GetByID loads a project aggregate by internal ID.
func (r *PoolRepo) GetByID(ctx context.Context, id shared.ProjectID) (*pool.Project, error) {
	row, err := r.q.GetProjectByID(ctx, projectID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: get project: %w", err)
	}
	return projectFromRow(row), nil
}

// ListByDomain returns projects for a KI domain, optionally filtered by state.
func (r *PoolRepo) ListByDomain(ctx context.Context, kiDomainID string, state *pool.State) ([]pool.Project, error) {
	var domainParam *string
	if kiDomainID != "" {
		domainParam = &kiDomainID
	}
	var stateParam *string
	if state != nil {
		v := string(*state)
		stateParam = &v
	}
	rows, err := r.q.ListProjectsByDomain(ctx, sqlcgen.ListProjectsByDomainParams{
		KiDomainID: domainParam,
		State:      stateParam,
	})
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: list projects: %w", err)
	}
	return projectsFromRows(rows), nil
}

// SeedInsert inserts pre-created KI projects idempotently by ki_project_id.
func (r *PoolRepo) SeedInsert(ctx context.Context, projects []pool.Project) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("pgxrepo: nil pool")
	}
	return pgx.BeginTxFunc(ctx, r.db, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		for i := range projects {
			p := projects[i]
			cleanupFailures, err := cleanupFailuresInt32(p.CleanupFailures)
			if err != nil {
				return err
			}
			_, err = q.SeedInsertProject(ctx, sqlcgen.SeedInsertProjectParams{
				ID:                projectID(p.ID),
				KiProjectID:       p.KIProjectID,
				KiDomainID:        p.KIDomainID,
				Name:              p.Name,
				State:             stateOrFree(p.State),
				AllocatedToLabID:  nullableAllocatedLabIDForState(&p),
				CleanupFailures:   cleanupFailures,
				LastStateChangeAt: nullableTimeArg(p.LastStateChangeAt),
				CreatedAt:         nullableTimeArg(p.CreatedAt),
			})
			if err != nil {
				return fmt.Errorf("pgxrepo: seed project %s: %w", p.KIProjectID, err)
			}
		}
		return nil
	})
}

func (r *PoolRepo) queriesInTx(tx ports.Tx) (*sqlcgen.Queries, error) {
	pgxTx, err := txFromPort(tx)
	if err != nil {
		return nil, err
	}
	return r.q.WithTx(pgxTx), nil
}

func (r *PoolRepo) appendProjectEvent(ctx context.Context, q *sqlcgen.Queries, ev pool.DomainEvent, occurredAt time.Time) error {
	payload, subjectID, err := projectEventPayload(ev)
	if err != nil {
		return err
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("pgxrepo: marshal project event: %w", err)
	}

	occurred := nullableTimeArg(occurredAt)
	if err := q.InsertOutbox(ctx, sqlcgen.InsertOutboxParams{
		Topic:      ev.Kind(),
		Payload:    payloadBytes,
		OccurredAt: occurred,
	}); err != nil {
		return fmt.Errorf("pgxrepo: insert outbox: %w", err)
	}
	if err := q.InsertAuditEvent(ctx, sqlcgen.InsertAuditEventParams{
		ID:          uuid.New(),
		Kind:        ev.Kind(),
		SubjectType: "project",
		SubjectID:   &subjectID,
		Payload:     payloadBytes,
		Column7:     "",
		OccurredAt:  occurred,
	}); err != nil {
		return fmt.Errorf("pgxrepo: insert audit event: %w", err)
	}
	return nil
}

func nullableAllocatedLabIDForState(p *pool.Project) uuid.NullUUID {
	if p.State == pool.StateAllocated || p.State == pool.StateCleaning {
		return nullableLabID(p.AllocatedToLabID)
	}
	return uuid.NullUUID{}
}

func nullableTimeArg(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

func stateOrFree(state pool.State) string {
	if state == "" {
		return string(pool.StateFree)
	}
	return string(state)
}

func cleanupFailuresInt32(n int) (int32, error) {
	const maxInt32 = int(^uint32(0) >> 1)
	if n < 0 || n > maxInt32 {
		return 0, shared.ErrInvalidInput
	}
	return int32(n), nil
}

func projectEventPayload(ev pool.DomainEvent) (map[string]interface{}, string, error) {
	switch e := ev.(type) {
	case pool.EventAllocated:
		projectID := e.ProjectID.String()
		return map[string]interface{}{
			"project_id": projectID,
			"lab_id":     e.LabID.String(),
		}, projectID, nil
	case pool.EventReleasing:
		projectID := e.ProjectID.String()
		return map[string]interface{}{
			"project_id": projectID,
			"lab_id":     e.LabID.String(),
		}, projectID, nil
	case pool.EventFreed:
		projectID := e.ProjectID.String()
		return map[string]interface{}{
			"project_id": projectID,
		}, projectID, nil
	case pool.EventQuarantined:
		projectID := e.ProjectID.String()
		return map[string]interface{}{
			"project_id": projectID,
			"reason":     e.Reason,
			"failures":   e.Failures,
		}, projectID, nil
	default:
		return nil, "", fmt.Errorf("pgxrepo: unsupported project event %T", ev)
	}
}
