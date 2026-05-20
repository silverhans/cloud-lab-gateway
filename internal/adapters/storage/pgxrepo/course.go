package pgxrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloud-lab-gateway/gateway/internal/adapters/storage/sqlcgen"
	"github.com/cloud-lab-gateway/gateway/internal/domain/identity"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CourseRepo persists courses and enrollments in Postgres.
type CourseRepo struct {
	q *sqlcgen.Queries
}

var _ ports.CourseRepo = (*CourseRepo)(nil)

// NewCourseRepo creates a CourseRepo backed by a pgx pool.
func NewCourseRepo(db *pgxpool.Pool) *CourseRepo {
	return &CourseRepo{q: sqlcgen.New(db)}
}

// GetByID loads a course by internal ID.
func (r *CourseRepo) GetByID(ctx context.Context, id shared.CourseID) (*identity.Course, error) {
	row, err := r.q.GetCourseByID(ctx, courseID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: get course: %w", err)
	}
	return courseFromRow(row), nil
}

// GetByExternalID loads a course by Moodle/LMS external ID.
func (r *CourseRepo) GetByExternalID(ctx context.Context, externalID string) (*identity.Course, error) {
	row, err := r.q.GetCourseByExternalID(ctx, externalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: get course by external id: %w", err)
	}
	return courseFromRow(row), nil
}

// Upsert inserts or updates a course by external ID.
func (r *CourseRepo) Upsert(ctx context.Context, tx ports.Tx, c *identity.Course) error {
	if c == nil {
		return shared.ErrInvalidInput
	}
	q, err := r.queriesInTx(tx)
	if err != nil {
		return err
	}
	ensureCourseID(c)
	if err := q.UpsertCourse(ctx, sqlcgen.UpsertCourseParams{
		ID:         courseID(c.ID),
		ExternalID: c.ExternalID,
		Name:       c.Name,
		KiDomainID: c.KIDomainID,
		CreatedAt:  nullableTimeArg(c.CreatedAt),
	}); err != nil {
		return fmt.Errorf("pgxrepo: upsert course: %w", err)
	}
	return nil
}

// Enroll idempotently assigns a user role inside a course.
func (r *CourseRepo) Enroll(ctx context.Context, tx ports.Tx, e identity.Enrollment) error {
	q, err := r.queriesInTx(tx)
	if err != nil {
		return err
	}
	if err := q.EnrollCourseUser(ctx, sqlcgen.EnrollCourseUserParams{
		UserID:       userID(e.UserID),
		CourseID:     courseID(e.CourseID),
		RoleInCourse: string(e.RoleInCourse),
		CreatedAt:    nullableTimeArg(e.CreatedAt),
	}); err != nil {
		return fmt.Errorf("pgxrepo: enroll course user: %w", err)
	}
	return nil
}

func (r *CourseRepo) queriesInTx(tx ports.Tx) (*sqlcgen.Queries, error) {
	pgxTx, err := txFromPort(tx)
	if err != nil {
		return nil, err
	}
	return r.q.WithTx(pgxTx), nil
}

func courseFromRow(row sqlcgen.Course) *identity.Course {
	return &identity.Course{
		ID:         shared.CourseID(row.ID),
		ExternalID: row.ExternalID,
		Name:       row.Name,
		KIDomainID: row.KiDomainID,
		CreatedAt:  timeFromTimestamptz(row.CreatedAt),
	}
}

func ensureCourseID(c *identity.Course) {
	if c.ID == (shared.CourseID{}) {
		c.ID = shared.CourseID(uuid.New())
	}
}
