package httpapi

import (
	appadmin "github.com/cloud-lab-gateway/gateway/internal/app/admin"
	applab "github.com/cloud-lab-gateway/gateway/internal/app/lab"
	appverify "github.com/cloud-lab-gateway/gateway/internal/app/verify"
	"github.com/cloud-lab-gateway/gateway/internal/domain/identity"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

func labActor(p Principal) applab.Actor {
	return applab.Actor{
		UserID:      shared.UserID(p.UserID),
		Role:        identity.Role(p.Role),
		CourseRoles: courseRoles(p),
	}
}

func verifyActor(p Principal) appverify.Actor {
	return appverify.Actor{
		UserID:      shared.UserID(p.UserID),
		Role:        identity.Role(p.Role),
		CourseRoles: courseRoles(p),
	}
}

func adminActor(p Principal) appadmin.Actor {
	return appadmin.Actor{
		UserID: shared.UserID(p.UserID),
		Role:   identity.Role(p.Role),
	}
}

func courseRoles(p Principal) map[shared.CourseID]identity.CourseRole {
	if len(p.CourseRoles) == 0 {
		return nil
	}
	out := make(map[shared.CourseID]identity.CourseRole, len(p.CourseRoles))
	for id, role := range p.CourseRoles {
		out[shared.CourseID(id)] = identity.CourseRole(role)
	}
	return out
}
