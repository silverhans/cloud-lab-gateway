package httpapi

import (
	"context"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	verifydomain "github.com/cloud-lab-gateway/gateway/internal/domain/verify"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (s *Server) GetLabsIdChecks(ctx context.Context, request GetLabsIdChecksRequestObject) (GetLabsIdChecksResponseObject, error) {
	if s.deps.Verify == nil {
		return nil, s.notImplemented()
	}
	principal, ok := principalFrom(ctx)
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	runs, err := s.deps.Verify.ListChecksByLab(ctx, verifyActor(principal), shared.LabInstanceID(request.Id))
	if err != nil {
		return nil, err
	}
	out := make([]CheckRun, 0, len(runs))
	for i := range runs {
		out = append(out, checkRunResponse(&runs[i]))
	}
	return GetLabsIdChecks200JSONResponse(out), nil
}

func (s *Server) PostLabsIdChecks(ctx context.Context, request PostLabsIdChecksRequestObject) (PostLabsIdChecksResponseObject, error) {
	if s.deps.Verify == nil || request.Body == nil || request.Body.CheckTemplateId == nilUUID {
		return nil, shared.ErrInvalidInput
	}
	principal, ok := principalFrom(ctx)
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	run, err := s.deps.Verify.RunCheck(ctx,
		verifyActor(principal),
		shared.LabInstanceID(request.Id),
		shared.CheckTemplate(request.Body.CheckTemplateId),
	)
	if err != nil {
		return nil, err
	}
	return PostLabsIdChecks202JSONResponse(checkRunResponse(run)), nil
}

func (s *Server) GetChecksId(ctx context.Context, request GetChecksIdRequestObject) (GetChecksIdResponseObject, error) {
	if s.deps.Verify == nil {
		return nil, s.notImplemented()
	}
	principal, ok := principalFrom(ctx)
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	run, err := s.deps.Verify.GetCheckRun(ctx, verifyActor(principal), shared.CheckRunID(request.Id))
	if err != nil {
		return nil, err
	}
	return GetChecksId200JSONResponse(checkRunDetailResponse(run)), nil
}

func checkRunResponse(run *verifydomain.CheckRun) CheckRun {
	state := CheckRunState(run.State)
	summary := run.Summary
	return CheckRun{
		Id:              uuidPtr(openapi_types.UUID(run.ID)),
		LabInstanceId:   uuidPtr(openapi_types.UUID(run.LabInstanceID)),
		CheckTemplateId: uuidPtr(openapi_types.UUID(run.CheckTemplateID)),
		State:           &state,
		Summary:         &summary,
		StartedAt:       run.StartedAt,
		FinishedAt:      run.FinishedAt,
	}
}

func checkRunDetailResponse(run *verifydomain.CheckRun) CheckRunDetail {
	base := checkRunResponse(run)
	state := CheckRunDetailState(run.State)
	stdout := run.AnsibleStdout
	detail := CheckRunDetail{
		Id:              base.Id,
		LabInstanceId:   base.LabInstanceId,
		CheckTemplateId: base.CheckTemplateId,
		State:           &state,
		Summary:         base.Summary,
		StartedAt:       base.StartedAt,
		FinishedAt:      base.FinishedAt,
		AnsibleStdout:   &stdout,
	}
	if len(run.Steps) > 0 {
		steps := make([]struct {
			Actual   *interface{}               `json:"actual,omitempty"`
			Expected *interface{}               `json:"expected,omitempty"`
			Message  *string                    `json:"message,omitempty"`
			Status   *CheckRunDetailStepsStatus `json:"status,omitempty"`
			TaskName *string                    `json:"task_name,omitempty"`
		}, 0, len(run.Steps))
		for _, step := range run.Steps {
			status := CheckRunDetailStepsStatus(step.Status)
			taskName := step.TaskName
			message := step.Message
			actual := step.Actual
			expected := step.Expected
			steps = append(steps, struct {
				Actual   *interface{}               `json:"actual,omitempty"`
				Expected *interface{}               `json:"expected,omitempty"`
				Message  *string                    `json:"message,omitempty"`
				Status   *CheckRunDetailStepsStatus `json:"status,omitempty"`
				TaskName *string                    `json:"task_name,omitempty"`
			}{
				Actual:   &actual,
				Expected: &expected,
				Message:  &message,
				Status:   &status,
				TaskName: &taskName,
			})
		}
		detail.Steps = &steps
	}
	return detail
}

func uuidPtr(id openapi_types.UUID) *openapi_types.UUID {
	return &id
}
