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

// UserRepo persists users and LTI identities in Postgres.
type UserRepo struct {
	q *sqlcgen.Queries
}

var _ ports.UserRepo = (*UserRepo)(nil)

// NewUserRepo creates a UserRepo backed by a pgx pool.
func NewUserRepo(db *pgxpool.Pool) *UserRepo {
	return &UserRepo{q: sqlcgen.New(db)}
}

func (r *UserRepo) GetByID(ctx context.Context, id shared.UserID) (*identity.User, error) {
	row, err := r.q.GetUserByID(ctx, userID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: get user: %w", err)
	}
	return userFromRow(row), nil
}

func (r *UserRepo) GetByLTI(ctx context.Context, iss, sub string) (*identity.User, error) {
	row, err := r.q.GetUserByLTI(ctx, sqlcgen.GetUserByLTIParams{Iss: iss, Sub: sub})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: get user by lti: %w", err)
	}
	return userFromRow(row), nil
}

func (r *UserRepo) UpsertFromLaunch(ctx context.Context, tx ports.Tx, iss, sub, displayName, email string, role identity.Role) (*identity.User, error) {
	if iss == "" || sub == "" || displayName == "" || role == "" {
		return nil, shared.ErrInvalidInput
	}
	q, err := r.queriesInTx(tx)
	if err != nil {
		return nil, err
	}

	existing, err := q.GetUserByLTI(ctx, sqlcgen.GetUserByLTIParams{Iss: iss, Sub: sub})
	if err == nil {
		row, err := q.UpdateUserFromLaunch(ctx, sqlcgen.UpdateUserFromLaunchParams{
			ID:          existing.ID,
			DisplayName: displayName,
			Email:       nullableEmail(email),
			Role:        string(role),
		})
		if err != nil {
			return nil, fmt.Errorf("pgxrepo: update user from launch: %w", err)
		}
		return userFromRow(row), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("pgxrepo: get user by lti in upsert: %w", err)
	}

	row, err := q.InsertUserFromLaunch(ctx, sqlcgen.InsertUserFromLaunchParams{
		ID:          uuid.New(),
		DisplayName: displayName,
		Email:       nullableEmail(email),
		Role:        string(role),
	})
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: insert user from launch: %w", err)
	}
	if err := q.UpsertLTIIdentity(ctx, sqlcgen.UpsertLTIIdentityParams{
		UserID: row.ID,
		Iss:    iss,
		Sub:    sub,
	}); err != nil {
		return nil, fmt.Errorf("pgxrepo: upsert lti identity: %w", err)
	}
	return userFromRow(row), nil
}

func (r *UserRepo) GetCourseRoles(ctx context.Context, id shared.UserID) (map[shared.CourseID]identity.CourseRole, error) {
	rows, err := r.q.ListCourseRolesByUser(ctx, userID(id))
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: list course roles: %w", err)
	}
	out := make(map[shared.CourseID]identity.CourseRole, len(rows))
	for _, row := range rows {
		out[shared.CourseID(row.CourseID)] = identity.CourseRole(row.RoleInCourse)
	}
	return out, nil
}

func (r *UserRepo) queriesInTx(tx ports.Tx) (*sqlcgen.Queries, error) {
	pgxTx, err := txFromPort(tx)
	if err != nil {
		return nil, err
	}
	return r.q.WithTx(pgxTx), nil
}

func userFromRow(row sqlcgen.User) *identity.User {
	return &identity.User{
		ID:          shared.UserID(row.ID),
		DisplayName: row.DisplayName,
		Email:       stringFromPtr(row.Email),
		Role:        identity.Role(row.Role),
		CreatedAt:   timeFromTimestamptz(row.CreatedAt),
	}
}

func nullableEmail(email string) *string {
	if email == "" {
		return nil
	}
	return &email
}
