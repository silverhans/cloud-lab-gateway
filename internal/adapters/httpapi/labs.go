package httpapi

import (
	"context"

	applab "github.com/cloud-lab-gateway/gateway/internal/app/lab"
	labdomain "github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (s *Server) PostLabs(ctx context.Context, request PostLabsRequestObject) (PostLabsResponseObject, error) {
	if s.deps.Lab == nil || request.Body == nil {
		return nil, shared.ErrInvalidInput
	}
	principal, ok := principalFrom(ctx)
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	if request.Body.CourseId == nilUUID || request.Body.LabTemplateId == nilUUID {
		return nil, shared.ErrInvalidInput
	}
	inst, err := s.deps.Lab.CreateLab(ctx, applab.CreateInput{
		StudentUserID: shared.UserID(principal.UserID),
		CourseID:      shared.CourseID(request.Body.CourseId),
		LabTemplateID: shared.LabTemplateID(request.Body.LabTemplateId),
		RequestID:     requestID(ctx),
	})
	if err != nil {
		return nil, err
	}
	return PostLabs201JSONResponse(labResponse(inst)), nil
}

func labResponse(l *labdomain.LabInstance) LabInstance {
	out := LabInstance{
		Id:            openapi_types.UUID(l.ID),
		StudentUserId: openapi_types.UUID(l.StudentUserID),
		CourseId:      openapi_types.UUID(l.CourseID),
		LabTemplateId: openapi_types.UUID(l.LabTemplateID),
		State:         LabState(l.State),
		CreatedAt:     l.CreatedAt,
		UpdatedAt:     timePtr(l.UpdatedAt),
		CleanupAt:     l.CleanupAt,
		UnfreezeAt:    l.UnfreezeAt,
	}
	if l.ProjectID != nil {
		v := openapi_types.UUID(*l.ProjectID)
		out.ProjectId = &v
	}
	if l.StateReason != "" {
		out.StateReason = &l.StateReason
	}
	return out
}
