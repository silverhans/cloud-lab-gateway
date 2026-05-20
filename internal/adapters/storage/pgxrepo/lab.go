package pgxrepo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/adapters/storage/sqlcgen"
	"github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LabRepo persists lab.LabInstance aggregates in Postgres.
type LabRepo struct {
	q *sqlcgen.Queries
}

var _ ports.LabRepo = (*LabRepo)(nil)

// NewLabRepo creates a LabRepo backed by a pgx pool.
func NewLabRepo(db *pgxpool.Pool) *LabRepo {
	return &LabRepo{q: sqlcgen.New(db)}
}

// Create inserts a LabInstance and flushes pending domain events.
func (r *LabRepo) Create(ctx context.Context, tx ports.Tx, l *lab.LabInstance) error {
	if l == nil {
		return shared.ErrInvalidInput
	}
	q, err := r.queriesInTx(tx)
	if err != nil {
		return err
	}
	params, err := createLabParams(l)
	if err != nil {
		return err
	}
	if err := q.CreateLabInstance(ctx, params); isUniqueViolation(err) {
		return shared.ErrLabAlreadyActive
	} else if err != nil {
		return fmt.Errorf("pgxrepo: create lab: %w", err)
	}

	return r.flushLabEvents(ctx, q, l, labEventOccurredAt(l))
}

// Save persists a LabInstance state and flushes pending domain events.
func (r *LabRepo) Save(ctx context.Context, tx ports.Tx, l *lab.LabInstance) error {
	if l == nil {
		return shared.ErrInvalidInput
	}
	q, err := r.queriesInTx(tx)
	if err != nil {
		return err
	}
	params, err := updateLabParams(l)
	if err != nil {
		return err
	}
	if _, err := q.UpdateLabInstance(ctx, params); errors.Is(err, pgx.ErrNoRows) {
		return shared.ErrIdempotencyClash
	} else if err != nil {
		return fmt.Errorf("pgxrepo: save lab: %w", err)
	}

	return r.flushLabEvents(ctx, q, l, labEventOccurredAt(l))
}

// GetByID loads a LabInstance aggregate by ID.
func (r *LabRepo) GetByID(ctx context.Context, id shared.LabInstanceID) (*lab.LabInstance, error) {
	row, err := r.q.GetLabInstanceByID(ctx, labID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: get lab: %w", err)
	}
	return labFromRow(row)
}

// FindActiveByStudentAndCourse returns nil when no active lab exists.
func (r *LabRepo) FindActiveByStudentAndCourse(ctx context.Context, studentID shared.UserID, courseIDValue shared.CourseID) (*lab.LabInstance, error) {
	row, err := r.q.FindActiveLabByStudentAndCourse(ctx, sqlcgen.FindActiveLabByStudentAndCourseParams{
		StudentUserID: userID(studentID),
		CourseID:      courseID(courseIDValue),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: find active lab: %w", err)
	}
	return labFromRow(row)
}

// ListPendingCleanup returns ready labs whose cleanup timer is overdue.
func (r *LabRepo) ListPendingCleanup(ctx context.Context, before time.Time, limit int) ([]lab.LabInstance, error) {
	limit32, err := listLimitInt32(limit)
	if err != nil {
		return nil, err
	}
	if limit32 == 0 {
		return []lab.LabInstance{}, nil
	}
	rows, err := r.q.ListPendingCleanupLabs(ctx, sqlcgen.ListPendingCleanupLabsParams{
		CleanupAt: timestamptz(before),
		Limit:     limit32,
	})
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: list pending cleanup labs: %w", err)
	}
	return labsFromRows(rows)
}

// ListPendingUnfreeze returns frozen labs whose unfreeze timer is overdue.
func (r *LabRepo) ListPendingUnfreeze(ctx context.Context, before time.Time, limit int) ([]lab.LabInstance, error) {
	limit32, err := listLimitInt32(limit)
	if err != nil {
		return nil, err
	}
	if limit32 == 0 {
		return []lab.LabInstance{}, nil
	}
	rows, err := r.q.ListPendingUnfreezeLabs(ctx, sqlcgen.ListPendingUnfreezeLabsParams{
		UnfreezeAt: timestamptz(before),
		Limit:      limit32,
	})
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: list pending unfreeze labs: %w", err)
	}
	return labsFromRows(rows)
}

func (r *LabRepo) queriesInTx(tx ports.Tx) (*sqlcgen.Queries, error) {
	pgxTx, err := txFromPort(tx)
	if err != nil {
		return nil, err
	}
	return r.q.WithTx(pgxTx), nil
}

func (r *LabRepo) flushLabEvents(ctx context.Context, q *sqlcgen.Queries, l *lab.LabInstance, occurredAt time.Time) error {
	for _, ev := range l.PullEvents() {
		if err := r.appendLabEvent(ctx, q, ev, occurredAt); err != nil {
			return err
		}
	}
	return nil
}

func (r *LabRepo) appendLabEvent(ctx context.Context, q *sqlcgen.Queries, ev lab.DomainEvent, occurredAt time.Time) error {
	payload, subjectID, err := labEventPayload(ev)
	if err != nil {
		return err
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("pgxrepo: marshal lab event: %w", err)
	}

	occurred := nullableTimeArg(occurredAt)
	if err := q.InsertOutbox(ctx, sqlcgen.InsertOutboxParams{
		Topic:      ev.Kind(),
		Payload:    payloadBytes,
		OccurredAt: occurred,
	}); err != nil {
		return fmt.Errorf("pgxrepo: insert lab outbox: %w", err)
	}
	if err := q.InsertAuditEvent(ctx, sqlcgen.InsertAuditEventParams{
		ID:          uuid.New(),
		Kind:        ev.Kind(),
		SubjectType: "lab_instance",
		SubjectID:   &subjectID,
		Payload:     payloadBytes,
		Column7:     "",
		OccurredAt:  occurred,
	}); err != nil {
		return fmt.Errorf("pgxrepo: insert lab audit event: %w", err)
	}
	return nil
}

func labEventPayload(ev lab.DomainEvent) (map[string]interface{}, string, error) {
	switch e := ev.(type) {
	case lab.EventCreated:
		labIDValue := e.LabID.String()
		return map[string]interface{}{
			"lab_id":      labIDValue,
			"student_id":  e.StudentID.String(),
			"course_id":   e.CourseID.String(),
			"template_id": e.TemplateID.String(),
		}, labIDValue, nil
	case lab.EventStateChanged:
		labIDValue := e.LabID.String()
		return map[string]interface{}{
			"lab_id": labIDValue,
			"from":   string(e.From),
			"to":     string(e.To),
			"reason": e.Reason,
		}, labIDValue, nil
	case lab.EventProjectAssigned:
		labIDValue := e.LabID.String()
		return map[string]interface{}{
			"lab_id":     labIDValue,
			"project_id": e.ProjectID.String(),
		}, labIDValue, nil
	case lab.EventCleanupExtended:
		labIDValue := e.LabID.String()
		return map[string]interface{}{
			"lab_id":         labIDValue,
			"new_cleanup_at": e.NewCleanupAt,
		}, labIDValue, nil
	default:
		return nil, "", fmt.Errorf("pgxrepo: unsupported lab event %T", ev)
	}
}

func labEventOccurredAt(l *lab.LabInstance) time.Time {
	if !l.UpdatedAt.IsZero() {
		return l.UpdatedAt
	}
	return l.CreatedAt
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func listLimitInt32(limit int) (int32, error) {
	const maxInt32 = int(^uint32(0) >> 1)
	if limit < 0 || limit > maxInt32 {
		return 0, shared.ErrInvalidInput
	}
	return int32(limit), nil
}
