package identity

import (
	"context"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

// Action is a fine-grained verb the policy authorises.
type Action string

const (
	ActionLabCreate    Action = "lab.create"
	ActionLabRead      Action = "lab.read"
	ActionLabFreeze    Action = "lab.freeze"
	ActionLabUnfreeze  Action = "lab.unfreeze"
	ActionLabExtend    Action = "lab.extend"
	ActionLabDelete    Action = "lab.delete"
	ActionLabSSHKey    Action = "lab.ssh_key"
	ActionCheckRun     Action = "check.run"
	ActionAdminPool    Action = "admin.pool"
	ActionAdminSetting Action = "admin.setting"
	ActionAdminQuota   Action = "admin.quota"
	ActionAdminAudit   Action = "admin.audit"
)

// Resource is the target of an Action. Implementations carry just enough
// context to authorise — typically aggregate ID plus ownership.
type Resource interface {
	ResourceKind() string
}

// LabResource is the owning context for any lab-related action.
type LabResource struct {
	LabID    shared.LabInstanceID
	OwnerID  shared.UserID
	CourseID shared.CourseID
}

func (LabResource) ResourceKind() string { return "lab_instance" }

type CourseResource struct {
	CourseID shared.CourseID
}

func (CourseResource) ResourceKind() string { return "course" }

type GlobalAdminResource struct{}

func (GlobalAdminResource) ResourceKind() string { return "admin" }

// Subject is the identity asking to perform Action.
type Subject struct {
	UserID      shared.UserID
	Role        Role
	CourseRoles map[shared.CourseID]CourseRole // populated by app layer
}

// Policy decides authorisation. Pure function with no I/O at the domain layer;
// the application layer wraps it with audit logging on deny.
type Policy interface {
	Can(ctx context.Context, subj Subject, action Action, res Resource) error
}

// DefaultPolicy implements straightforward RBAC matching the spec.
type DefaultPolicy struct{}

func (DefaultPolicy) Can(ctx context.Context, subj Subject, action Action, res Resource) error {
	switch action {
	case ActionLabCreate, ActionLabRead, ActionLabFreeze, ActionLabExtend, ActionLabUnfreeze,
		ActionLabDelete, ActionLabSSHKey, ActionCheckRun:
		return canActOnLab(subj, action, res)
	case ActionAdminPool, ActionAdminAudit, ActionAdminQuota:
		if subj.Role == RoleAdmin {
			return nil
		}
		return shared.ErrForbidden
	case ActionAdminSetting:
		if subj.Role == RoleAdmin || subj.Role == RoleTeacher {
			return nil
		}
		return shared.ErrForbidden
	}
	return shared.ErrForbidden
}

func canActOnLab(subj Subject, action Action, res Resource) error {
	lab, ok := res.(LabResource)
	if !ok {
		return shared.ErrForbidden
	}
	if subj.Role == RoleAdmin {
		return nil
	}
	// Teacher of the course can do everything except download student's SSH key.
	if subj.CourseRoles[lab.CourseID] == CourseRoleTeacher {
		if action == ActionLabSSHKey {
			return shared.ErrForbidden
		}
		return nil
	}
	// Student must be the owner.
	if lab.OwnerID != subj.UserID {
		return shared.ErrForbidden
	}
	// Students cannot unfreeze (only report).
	if action == ActionLabUnfreeze || action == ActionLabExtend {
		return shared.ErrForbidden
	}
	return nil
}
