//go:build integration

package pgxrepo

import (
	"context"
	"testing"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/identity"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/google/uuid"
)

func TestCourseRepoUpsertAndGet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	repo := NewCourseRepo(db)
	uow := NewUoW(db)
	course := &identity.Course{
		ID:         shared.CourseID(uuid.New()),
		ExternalID: "external-" + uuid.NewString(),
		Name:       "Linux Basics",
		KIDomainID: "domain-" + uuid.NewString(),
		CreatedAt:  time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC),
	}

	if err := uow.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		return repo.Upsert(ctx, tx, course)
	}); err != nil {
		t.Fatalf("upsert course: %v", err)
	}

	byID, err := repo.GetByID(ctx, course.ID)
	if err != nil {
		t.Fatalf("get course by id: %v", err)
	}
	byExternal, err := repo.GetByExternalID(ctx, course.ExternalID)
	if err != nil {
		t.Fatalf("get course by external id: %v", err)
	}
	if byID.ID != course.ID || byExternal.ID != course.ID || byID.KIDomainID != course.KIDomainID {
		t.Fatalf("course round trip mismatch: byID=%+v byExternal=%+v want=%+v", byID, byExternal, course)
	}
}

func TestCourseRepoEnrollIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	repo := NewCourseRepo(db)
	uow := NewUoW(db)
	userID := insertTestUser(t, db, identity.RoleStudent)
	course := &identity.Course{
		ID:         shared.CourseID(uuid.New()),
		ExternalID: "external-" + uuid.NewString(),
		Name:       "Networking",
		KIDomainID: "domain-" + uuid.NewString(),
	}
	if err := uow.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		return repo.Upsert(ctx, tx, course)
	}); err != nil {
		t.Fatalf("upsert course: %v", err)
	}

	for _, role := range []identity.CourseRole{identity.CourseRoleLearner, identity.CourseRoleTeacher} {
		role := role
		if err := uow.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
			return repo.Enroll(ctx, tx, identity.Enrollment{
				UserID:       userID,
				CourseID:     course.ID,
				RoleInCourse: role,
			})
		}); err != nil {
			t.Fatalf("enroll role %s: %v", role, err)
		}
	}

	var got string
	if err := db.QueryRow(ctx,
		"SELECT role_in_course FROM enrollments WHERE user_id = $1 AND course_id = $2",
		uuid.UUID(userID), uuid.UUID(course.ID),
	).Scan(&got); err != nil {
		t.Fatalf("query enrollment: %v", err)
	}
	if got != string(identity.CourseRoleTeacher) {
		t.Fatalf("expected idempotent enroll to update role to teacher, got %s", got)
	}
}
