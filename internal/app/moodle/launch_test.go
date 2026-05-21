package moodle

import (
	"context"
	"testing"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/identity"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/google/uuid"
)

func TestHandleLaunchMaterialisesUserCourseAndEnrollment(t *testing.T) {
	t.Parallel()
	userID := shared.UserID(uuid.New())
	roleCourseID := shared.CourseID(uuid.New())
	users := &fakeUserRepo{
		user: &identity.User{
			ID:          userID,
			DisplayName: "Teacher One",
			Email:       "teacher@example.test",
			Role:        identity.RoleTeacher,
		},
		roles: map[shared.CourseID]identity.CourseRole{roleCourseID: identity.CourseRoleTeacher},
	}
	courses := &fakeCourseRepo{}
	deps := Deps{
		LMS: &fakeLMSProvider{launch: &ports.LTILaunch{
			Iss:              "https://moodle-emulator.local",
			Sub:              "user-teacher-001",
			Email:            "teacher@example.test",
			Name:             "Teacher One",
			CourseExternalID: "linux-101",
			ResourceLinkID:   "linux-users",
			RolesInContext:   []string{"http://purl.imsglobal.org/vocab/lis/v2/membership#Instructor"},
			Raw:              map[string]any{"context_title": "Linux basics"},
		}},
		UoW:     fakeUoW{},
		Users:   users,
		Courses: courses,
		Now:     func() time.Time { return time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC) },
	}

	result, err := HandleLaunch(context.Background(), deps, LaunchInput{
		IDToken: "signed.jwt",
		State:   "state-1",
		Nonce:   "nonce-1",
	})
	if err != nil {
		t.Fatalf("HandleLaunch: %v", err)
	}
	if users.gotRole != identity.RoleTeacher {
		t.Fatalf("user role = %s, want teacher", users.gotRole)
	}
	if courses.upserted.Name != "Linux basics" || courses.upserted.ExternalID != "linux-101" {
		t.Fatalf("course mismatch: %+v", courses.upserted)
	}
	if courses.enrolled.UserID != userID || courses.enrolled.RoleInCourse != identity.CourseRoleTeacher {
		t.Fatalf("enrollment mismatch: %+v", courses.enrolled)
	}
	if result.RedirectPath != "/teacher" || result.CourseRoles[roleCourseID] != identity.CourseRoleTeacher {
		t.Fatalf("result mismatch: %+v", result)
	}
}

type fakeLMSProvider struct {
	launch *ports.LTILaunch
	err    error
}

func (p *fakeLMSProvider) VerifyLaunch(context.Context, string, string, string) (*ports.LTILaunch, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.launch, nil
}

func (p *fakeLMSProvider) ReportGrade(context.Context, *ports.LTILaunch, float64, float64, string) error {
	return nil
}

func (p *fakeLMSProvider) GetCourseMembers(context.Context, string) ([]ports.LMSMember, error) {
	return nil, nil
}

type fakeUoW struct{}

func (fakeUoW) WithTx(ctx context.Context, fn func(context.Context, ports.Tx) error) error {
	return fn(ctx, fakeTx{})
}

type fakeTx struct{}

func (fakeTx) Private() {}

type fakeUserRepo struct {
	user    *identity.User
	roles   map[shared.CourseID]identity.CourseRole
	gotRole identity.Role
}

func (r *fakeUserRepo) GetByID(context.Context, shared.UserID) (*identity.User, error) {
	return r.user, nil
}

func (r *fakeUserRepo) GetByLTI(context.Context, string, string) (*identity.User, error) {
	return r.user, nil
}

func (r *fakeUserRepo) UpsertFromLaunch(_ context.Context, _ ports.Tx, _, _, displayName, email string, role identity.Role) (*identity.User, error) {
	r.gotRole = role
	r.user.DisplayName = displayName
	r.user.Email = email
	r.user.Role = role
	return r.user, nil
}

func (r *fakeUserRepo) GetCourseRoles(context.Context, shared.UserID) (map[shared.CourseID]identity.CourseRole, error) {
	return r.roles, nil
}

type fakeCourseRepo struct {
	upserted *identity.Course
	enrolled identity.Enrollment
}

func (r *fakeCourseRepo) GetByID(context.Context, shared.CourseID) (*identity.Course, error) {
	return r.upserted, nil
}

func (r *fakeCourseRepo) GetByExternalID(context.Context, string) (*identity.Course, error) {
	return r.upserted, nil
}

func (r *fakeCourseRepo) Upsert(_ context.Context, _ ports.Tx, c *identity.Course) error {
	if c.ID == (shared.CourseID{}) {
		c.ID = shared.CourseID(uuid.New())
	}
	copied := *c
	r.upserted = &copied
	return nil
}

func (r *fakeCourseRepo) Enroll(_ context.Context, _ ports.Tx, e identity.Enrollment) error {
	r.enrolled = e
	return nil
}
