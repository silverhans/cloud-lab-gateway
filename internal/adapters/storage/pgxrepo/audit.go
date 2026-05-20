package pgxrepo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloud-lab-gateway/gateway/internal/adapters/storage/sqlcgen"
	"github.com/cloud-lab-gateway/gateway/internal/domain/audit"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultAuditLimit = 100
	maxAuditLimit     = 500
)

// AuditRepo appends and queries audit events in Postgres.
type AuditRepo struct {
	q *sqlcgen.Queries
}

var _ ports.AuditRepo = (*AuditRepo)(nil)

// NewAuditRepo creates an AuditRepo backed by a pgx pool.
func NewAuditRepo(db *pgxpool.Pool) *AuditRepo {
	return &AuditRepo{q: sqlcgen.New(db)}
}

// Append writes an audit event outside a caller-owned transaction.
func (r *AuditRepo) Append(ctx context.Context, ev audit.AuditEvent) error {
	params, err := auditEventParams(ev)
	if err != nil {
		return err
	}
	if err := r.q.InsertAuditEvent(ctx, params); err != nil {
		return fmt.Errorf("pgxrepo: append audit event: %w", err)
	}
	return nil
}

// AppendInTx writes an audit event in the caller-owned transaction.
func (r *AuditRepo) AppendInTx(ctx context.Context, tx ports.Tx, ev audit.AuditEvent) error {
	q, err := r.queriesInTx(tx)
	if err != nil {
		return err
	}
	params, err := auditEventParams(ev)
	if err != nil {
		return err
	}
	if err := q.InsertAuditEvent(ctx, params); err != nil {
		return fmt.Errorf("pgxrepo: append audit event in tx: %w", err)
	}
	return nil
}

// Query returns audit events newest-first with a safe default limit.
func (r *AuditRepo) Query(ctx context.Context, f ports.AuditFilter) ([]audit.AuditEvent, error) {
	rows, err := r.q.QueryAuditEvents(ctx, sqlcgen.QueryAuditEventsParams{
		Kind:        f.Kind,
		ActorUserID: nullableAuditActorID(f.ActorUserID),
		Since:       timestamptzPtr(f.Since),
		Limit:       auditLimit(f.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: query audit events: %w", err)
	}
	return auditEventsFromRows(rows)
}

func (r *AuditRepo) queriesInTx(tx ports.Tx) (*sqlcgen.Queries, error) {
	pgxTx, err := txFromPort(tx)
	if err != nil {
		return nil, err
	}
	return r.q.WithTx(pgxTx), nil
}

func auditEventParams(ev audit.AuditEvent) (sqlcgen.InsertAuditEventParams, error) {
	payload, err := auditPayloadBytes(ev.Payload)
	if err != nil {
		return sqlcgen.InsertAuditEventParams{}, err
	}
	return sqlcgen.InsertAuditEventParams{
		ID:          auditEventID(ev.ID),
		Kind:        ev.Kind,
		ActorUserID: nullableAuditActorID(ev.ActorUserID),
		SubjectType: ev.SubjectType,
		SubjectID:   ev.SubjectID,
		Payload:     payload,
		Column7:     ev.RequestID,
		OccurredAt:  nullableTimeArg(ev.OccurredAt),
	}, nil
}

func auditPayloadBytes(payload interface{}) ([]byte, error) {
	if payload == nil {
		return []byte(`{}`), nil
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: marshal audit payload: %w", err)
	}
	return out, nil
}

func auditEventsFromRows(rows []sqlcgen.AuditEvent) ([]audit.AuditEvent, error) {
	out := make([]audit.AuditEvent, 0, len(rows))
	for _, row := range rows {
		ev, err := auditEventFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, nil
}

func auditEventFromRow(row sqlcgen.AuditEvent) (audit.AuditEvent, error) {
	var payload interface{} = map[string]interface{}{}
	if len(row.Payload) > 0 {
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			return audit.AuditEvent{}, fmt.Errorf("pgxrepo: unmarshal audit payload: %w", err)
		}
	}

	var actorID *shared.UserID
	if row.ActorUserID.Valid {
		v := shared.UserID(row.ActorUserID.UUID)
		actorID = &v
	}

	requestID := ""
	if row.RequestID != nil {
		requestID = *row.RequestID
	}

	return audit.AuditEvent{
		ID:          shared.AuditEventID(row.ID),
		Kind:        row.Kind,
		ActorUserID: actorID,
		SubjectType: row.SubjectType,
		SubjectID:   row.SubjectID,
		Payload:     payload,
		RequestID:   requestID,
		OccurredAt:  timeFromTimestamptz(row.OccurredAt),
	}, nil
}

func auditEventID(id shared.AuditEventID) uuid.UUID {
	if id == (shared.AuditEventID{}) {
		return uuid.New()
	}
	return uuid.UUID(id)
}

func nullableAuditActorID(id *shared.UserID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: uuid.UUID(*id), Valid: true}
}

func auditLimit(limit int) int32 {
	if limit <= 0 {
		return defaultAuditLimit
	}
	if limit > maxAuditLimit {
		return maxAuditLimit
	}
	return int32(limit)
}
