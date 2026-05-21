//go:build integration

package pgxrepo

import (
	"context"
	"testing"

	"github.com/cloud-lab-gateway/gateway/internal/domain/identity"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/google/uuid"
)

func TestUserRepoUpsertFromLaunchAndCourseRoles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	users := NewUserRepo(db)
	courses := NewCourseRepo(db)
	uow := NewUoW(db)

	var user *identity.User
	if err := uow.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		var err error
		user, err = users.UpsertFromLaunch(ctx, tx, "https://moodle.example.test", "user-1", "Student One", "student@example.test", identity.RoleStudent)
		return err
	}); err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	byLTI, err := users.GetByLTI(ctx, "https://moodle.example.test", "user-1")
	if err != nil {
		t.Fatalf("get by lti: %v", err)
	}
	if byLTI.ID != user.ID || byLTI.Email != "student@example.test" || byLTI.Role != identity.RoleStudent {
		t.Fatalf("unexpected user by lti: %+v", byLTI)
	}

	course := &identity.Course{
		ID:         shared.CourseID(uuid.New()),
		ExternalID: "course-" + uuid.NewString(),
		Name:       "Linux basics",
		KIDomainID: "domain-" + uuid.NewString(),
	}
	if err := uow.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		if err := courses.Upsert(ctx, tx, course); err != nil {
			return err
		}
		return courses.Enroll(ctx, tx, identity.Enrollment{
			UserID:       user.ID,
			CourseID:     course.ID,
			RoleInCourse: identity.CourseRoleLearner,
		})
	}); err != nil {
		t.Fatalf("enroll user: %v", err)
	}
	roles, err := users.GetCourseRoles(ctx, user.ID)
	if err != nil {
		t.Fatalf("get course roles: %v", err)
	}
	if roles[course.ID] != identity.CourseRoleLearner {
		t.Fatalf("course roles = %+v, want learner for %s", roles, course.ID)
	}
}
