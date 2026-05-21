package httpapi

import (
	"context"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (s *Server) GetAuthMe(ctx context.Context, _ GetAuthMeRequestObject) (GetAuthMeResponseObject, error) {
	p, ok := principalFrom(ctx)
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	email := optionalString(p.Email)
	return GetAuthMe200JSONResponse(UserMe{
		Id:          p.UserID,
		Role:        UserMeRole(p.Role),
		DisplayName: p.DisplayName,
		Email:       email,
		Courses:     userMeCourses(p),
	}), nil
}

func (s *Server) PostAuthLogout(ctx context.Context, _ PostAuthLogoutRequestObject) (PostAuthLogoutResponseObject, error) {
	w := responseWriterFromContext(ctx)
	if w != nil {
		clearSession(w, s.deps.DevMode)
	}
	return PostAuthLogout204Response{}, nil
}

func userMeCourses(p Principal) *[]struct {
	Id           *openapi_types.UUID        `json:"id,omitempty"`
	Name         *string                    `json:"name,omitempty"`
	RoleInCourse *UserMeCoursesRoleInCourse `json:"role_in_course,omitempty"`
} {
	if len(p.CourseRoles) == 0 {
		return nil
	}
	courses := make([]struct {
		Id           *openapi_types.UUID        `json:"id,omitempty"`
		Name         *string                    `json:"name,omitempty"`
		RoleInCourse *UserMeCoursesRoleInCourse `json:"role_in_course,omitempty"`
	}, 0, len(p.CourseRoles))
	for id, role := range p.CourseRoles {
		courseID := openapi_types.UUID(id)
		courseRole := UserMeCoursesRoleInCourse(role)
		courses = append(courses, struct {
			Id           *openapi_types.UUID        `json:"id,omitempty"`
			Name         *string                    `json:"name,omitempty"`
			RoleInCourse *UserMeCoursesRoleInCourse `json:"role_in_course,omitempty"`
		}{Id: &courseID, RoleInCourse: &courseRole})
	}
	return &courses
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
