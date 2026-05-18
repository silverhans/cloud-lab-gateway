package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

func studentSubject(uid shared.UserID, courseID shared.CourseID) Subject {
	return Subject{
		UserID: uid,
		Role:   RoleStudent,
		CourseRoles: map[shared.CourseID]CourseRole{
			courseID: CourseRoleLearner,
		},
	}
}

func teacherSubject(uid shared.UserID, courseID shared.CourseID) Subject {
	return Subject{
		UserID: uid,
		Role:   RoleTeacher,
		CourseRoles: map[shared.CourseID]CourseRole{
			courseID: CourseRoleTeacher,
		},
	}
}

func adminSubject(uid shared.UserID) Subject {
	return Subject{UserID: uid, Role: RoleAdmin}
}

func TestPolicy_StudentLifecycle(t *testing.T) {
	t.Parallel()
	pol := DefaultPolicy{}
	ctx := context.Background()

	owner := shared.NewUserID()
	course := shared.CourseID(shared.NewUserID())
	subj := studentSubject(owner, course)
	res := LabResource{LabID: shared.NewLabInstanceID(), OwnerID: owner, CourseID: course}

	allowed := []Action{ActionLabCreate, ActionLabRead, ActionLabFreeze, ActionLabDelete, ActionLabSSHKey, ActionCheckRun}
	for _, a := range allowed {
		a := a
		t.Run("allow_"+string(a), func(t *testing.T) {
			t.Parallel()
			if err := pol.Can(ctx, subj, a, res); err != nil {
				t.Errorf("student should be allowed %s on own lab, got %v", a, err)
			}
		})
	}

	denied := []Action{ActionLabUnfreeze, ActionLabExtend, ActionAdminPool, ActionAdminAudit, ActionAdminSetting}
	for _, a := range denied {
		a := a
		t.Run("deny_"+string(a), func(t *testing.T) {
			t.Parallel()
			if err := pol.Can(ctx, subj, a, res); !errors.Is(err, shared.ErrForbidden) {
				t.Errorf("student should be forbidden %s, got %v", a, err)
			}
		})
	}
}

func TestPolicy_StudentCannotTouchOtherStudentsLab(t *testing.T) {
	t.Parallel()
	pol := DefaultPolicy{}
	ctx := context.Background()

	courseID := shared.CourseID(shared.NewUserID())
	me := studentSubject(shared.NewUserID(), courseID)
	otherLab := LabResource{
		LabID:    shared.NewLabInstanceID(),
		OwnerID:  shared.NewUserID(), // someone else
		CourseID: courseID,
	}
	if err := pol.Can(ctx, me, ActionLabRead, otherLab); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("expected forbidden, got %v", err)
	}
}

func TestPolicy_TeacherCanManageCourseLabs(t *testing.T) {
	t.Parallel()
	pol := DefaultPolicy{}
	ctx := context.Background()

	courseID := shared.CourseID(shared.NewUserID())
	tch := teacherSubject(shared.NewUserID(), courseID)
	studentLab := LabResource{LabID: shared.NewLabInstanceID(), OwnerID: shared.NewUserID(), CourseID: courseID}

	for _, a := range []Action{ActionLabRead, ActionLabFreeze, ActionLabUnfreeze, ActionLabExtend, ActionLabDelete, ActionCheckRun} {
		a := a
		t.Run(string(a), func(t *testing.T) {
			t.Parallel()
			if err := pol.Can(ctx, tch, a, studentLab); err != nil {
				t.Errorf("teacher should be allowed %s, got %v", a, err)
			}
		})
	}

	// Teacher cannot download student's SSH key.
	if err := pol.Can(ctx, tch, ActionLabSSHKey, studentLab); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("teacher should not see student's SSH key, got %v", err)
	}
}

func TestPolicy_TeacherCannotAccessOtherCourse(t *testing.T) {
	t.Parallel()
	pol := DefaultPolicy{}
	ctx := context.Background()

	courseA := shared.CourseID(shared.NewUserID())
	courseB := shared.CourseID(shared.NewUserID())
	tch := teacherSubject(shared.NewUserID(), courseA)
	labInB := LabResource{LabID: shared.NewLabInstanceID(), OwnerID: shared.NewUserID(), CourseID: courseB}

	if err := pol.Can(ctx, tch, ActionLabRead, labInB); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("expected forbidden for cross-course access, got %v", err)
	}
}

func TestPolicy_AdminCanDoEverything(t *testing.T) {
	t.Parallel()
	pol := DefaultPolicy{}
	ctx := context.Background()

	admin := adminSubject(shared.NewUserID())
	lab := LabResource{LabID: shared.NewLabInstanceID(), OwnerID: shared.NewUserID(), CourseID: shared.CourseID(shared.NewUserID())}

	for _, a := range []Action{
		ActionLabCreate, ActionLabRead, ActionLabFreeze, ActionLabUnfreeze, ActionLabExtend, ActionLabDelete, ActionLabSSHKey,
		ActionCheckRun, ActionAdminPool, ActionAdminSetting, ActionAdminQuota, ActionAdminAudit,
	} {
		a := a
		t.Run(string(a), func(t *testing.T) {
			t.Parallel()
			res := Resource(GlobalAdminResource{})
			if a == ActionLabCreate || a == ActionLabRead || a == ActionLabFreeze ||
				a == ActionLabUnfreeze || a == ActionLabExtend || a == ActionLabDelete ||
				a == ActionLabSSHKey || a == ActionCheckRun {
				res = lab
			}
			if err := pol.Can(ctx, admin, a, res); err != nil {
				t.Errorf("admin should be allowed %s, got %v", a, err)
			}
		})
	}
}

func TestPolicy_TeacherCanEditSettings(t *testing.T) {
	t.Parallel()
	pol := DefaultPolicy{}
	tch := teacherSubject(shared.NewUserID(), shared.CourseID(shared.NewUserID()))
	if err := pol.Can(context.Background(), tch, ActionAdminSetting, GlobalAdminResource{}); err != nil {
		t.Errorf("teacher should be allowed to edit settings, got %v", err)
	}
}
