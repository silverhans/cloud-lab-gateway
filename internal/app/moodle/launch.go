package moodle

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloud-lab-gateway/gateway/internal/domain/identity"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

// LaunchInput is the browser POST from Moodle's LTI launch form.
type LaunchInput struct {
	IDToken string
	State   string
	Nonce   string
}

// LaunchResult is enough for the HTTP adapter to issue a browser session.
type LaunchResult struct {
	User         *identity.User
	CourseRoles  map[shared.CourseID]identity.CourseRole
	RedirectPath string
}

// HandleLaunch verifies an LTI launch and materialises user/course/enrollment.
func HandleLaunch(ctx context.Context, deps Deps, in LaunchInput) (*LaunchResult, error) {
	if deps.LMS == nil || deps.UoW == nil || deps.Users == nil || deps.Courses == nil || in.IDToken == "" {
		return nil, shared.ErrInvalidInput
	}
	launch, err := deps.LMS.VerifyLaunch(ctx, in.IDToken, in.State, in.Nonce)
	if err != nil {
		return nil, fmt.Errorf("moodle: verify launch: %w", err)
	}

	globalRole, courseRole := rolesFromLaunch(launch.RolesInContext)
	if globalRole == "" || courseRole == "" {
		return nil, shared.ErrForbidden
	}

	var user *identity.User
	var courseID shared.CourseID
	err = deps.UoW.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		u, err := deps.Users.UpsertFromLaunch(ctx, tx, launch.Iss, launch.Sub, launch.Name, launch.Email, globalRole)
		if err != nil {
			return err
		}
		user = u

		course := &identity.Course{
			ExternalID: launch.CourseExternalID,
			Name:       launch.CourseExternalID,
			KIDomainID: launch.CourseExternalID,
			CreatedAt:  deps.now(),
		}
		if title, ok := launch.Raw["context_title"].(string); ok && title != "" {
			course.Name = title
		}
		if err := deps.Courses.Upsert(ctx, tx, course); err != nil {
			return err
		}
		courseID = course.ID
		return deps.Courses.Enroll(ctx, tx, identity.Enrollment{
			UserID:       u.ID,
			CourseID:     courseID,
			RoleInCourse: courseRole,
			CreatedAt:    deps.now(),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("moodle: persist launch: %w", err)
	}

	roles, err := deps.Users.GetCourseRoles(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return &LaunchResult{
		User:         user,
		CourseRoles:  roles,
		RedirectPath: redirectForRole(globalRole),
	}, nil
}

func rolesFromLaunch(roles []string) (identity.Role, identity.CourseRole) {
	for _, role := range roles {
		lower := strings.ToLower(role)
		if strings.Contains(lower, "instructor") || strings.Contains(lower, "teacher") {
			return identity.RoleTeacher, identity.CourseRoleTeacher
		}
	}
	for _, role := range roles {
		if strings.Contains(strings.ToLower(role), "learner") {
			return identity.RoleStudent, identity.CourseRoleLearner
		}
	}
	return "", ""
}

func redirectForRole(role identity.Role) string {
	switch role {
	case identity.RoleTeacher:
		return "/teacher"
	case identity.RoleAdmin:
		return "/admin"
	default:
		return "/student"
	}
}
