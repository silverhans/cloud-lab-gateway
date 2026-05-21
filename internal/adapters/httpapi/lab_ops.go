package httpapi

import (
	"bytes"
	"context"
	"io"
	"time"

	applab "github.com/cloud-lab-gateway/gateway/internal/app/lab"
	labdomain "github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

func (s *Server) GetLabs(ctx context.Context, request GetLabsRequestObject) (GetLabsResponseObject, error) {
	if s.deps.LabOps == nil {
		return nil, s.notImplemented()
	}
	principal, ok := principalFrom(ctx)
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	var courseID *shared.CourseID
	if request.Params.CourseId != nil {
		v := shared.CourseID(*request.Params.CourseId)
		courseID = &v
	}
	states := make([]labdomain.State, 0)
	if request.Params.State != nil {
		for _, state := range *request.Params.State {
			states = append(states, labdomain.State(state))
		}
	}
	labs, err := s.deps.LabOps.ListLabs(ctx, labActor(principal), courseID, states)
	if err != nil {
		return nil, err
	}
	out := make([]LabInstance, 0, len(labs))
	for i := range labs {
		out = append(out, labResponse(&labs[i]))
	}
	return GetLabs200JSONResponse(out), nil
}

func (s *Server) GetLabsId(ctx context.Context, request GetLabsIdRequestObject) (GetLabsIdResponseObject, error) {
	detail, err := s.labDetail(ctx, request.Id)
	if err != nil {
		return nil, err
	}
	return GetLabsId200JSONResponse(labDetailResponse(detail)), nil
}

func (s *Server) DeleteLabsId(ctx context.Context, request DeleteLabsIdRequestObject) (DeleteLabsIdResponseObject, error) {
	if s.deps.LabOps == nil {
		return nil, s.notImplemented()
	}
	principal, ok := principalFrom(ctx)
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	lab, err := s.deps.LabOps.StopLab(ctx, applab.StopLabInput{
		Actor:     labActor(principal),
		LabID:     shared.LabInstanceID(request.Id),
		RequestID: requestID(ctx),
	})
	if err != nil {
		return nil, err
	}
	return DeleteLabsId202JSONResponse(labResponse(lab)), nil
}

func (s *Server) PostLabsIdFreeze(ctx context.Context, request PostLabsIdFreezeRequestObject) (PostLabsIdFreezeResponseObject, error) {
	if s.deps.LabOps == nil || request.Body == nil || request.Body.Reason == "" {
		return nil, shared.ErrInvalidInput
	}
	principal, ok := principalFrom(ctx)
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	lab, err := s.deps.LabOps.FreezeLab(ctx, applab.FreezeLabInput{
		Actor:            labActor(principal),
		LabID:            shared.LabInstanceID(request.Id),
		Reason:           request.Body.Reason,
		FreezeForSeconds: request.Body.FreezeForSeconds,
	})
	if err != nil {
		return nil, err
	}
	return PostLabsIdFreeze200JSONResponse(labResponse(lab)), nil
}

func (s *Server) PostLabsIdUnfreeze(ctx context.Context, request PostLabsIdUnfreezeRequestObject) (PostLabsIdUnfreezeResponseObject, error) {
	if s.deps.LabOps == nil {
		return nil, s.notImplemented()
	}
	principal, ok := principalFrom(ctx)
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	lab, err := s.deps.LabOps.UnfreezeLab(ctx, applab.UnfreezeLabInput{
		Actor: labActor(principal),
		LabID: shared.LabInstanceID(request.Id),
	})
	if err != nil {
		return nil, err
	}
	return PostLabsIdUnfreeze200JSONResponse(labResponse(lab)), nil
}

func (s *Server) PostLabsIdExtend(ctx context.Context, request PostLabsIdExtendRequestObject) (PostLabsIdExtendResponseObject, error) {
	if s.deps.LabOps == nil || request.Body == nil || request.Body.ExtendBySeconds <= 0 {
		return nil, shared.ErrInvalidInput
	}
	principal, ok := principalFrom(ctx)
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	lab, err := s.deps.LabOps.ExtendLab(ctx, applab.ExtendLabInput{
		Actor:    labActor(principal),
		LabID:    shared.LabInstanceID(request.Id),
		ExtendBy: time.Duration(request.Body.ExtendBySeconds) * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return PostLabsIdExtend200JSONResponse(labResponse(lab)), nil
}

func (s *Server) GetLabsIdSshKey(ctx context.Context, request GetLabsIdSshKeyRequestObject) (GetLabsIdSshKeyResponseObject, error) {
	if s.deps.LabOps == nil {
		return nil, s.notImplemented()
	}
	principal, ok := principalFrom(ctx)
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	key, err := s.deps.LabOps.GetLabSSHKey(ctx, labActor(principal), shared.LabInstanceID(request.Id))
	if err != nil {
		return nil, err
	}
	return GetLabsIdSshKey200ApplicationxPemFileResponse{
		Body:          newZeroizingReader(key),
		ContentLength: int64(len(key)),
	}, nil
}

func (s *Server) labDetail(ctx context.Context, id LabId) (*applab.LabDetail, error) {
	if s.deps.LabOps == nil {
		return nil, s.notImplemented()
	}
	principal, ok := principalFrom(ctx)
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	return s.deps.LabOps.GetLab(ctx, labActor(principal), shared.LabInstanceID(id))
}

func labDetailResponse(detail *applab.LabDetail) LabInstanceDetail {
	base := labResponse(detail.Lab)
	out := LabInstanceDetail{
		Id:            base.Id,
		StudentUserId: base.StudentUserId,
		CourseId:      base.CourseId,
		LabTemplateId: base.LabTemplateId,
		ProjectId:     base.ProjectId,
		State:         base.State,
		StateReason:   base.StateReason,
		CleanupAt:     base.CleanupAt,
		UnfreezeAt:    base.UnfreezeAt,
		CreatedAt:     base.CreatedAt,
		UpdatedAt:     base.UpdatedAt,
	}
	resources := detail.Lab.KIResources
	if resources.NetworkID != "" || len(resources.ServerIDs) > 0 || len(resources.FloatingIPs) > 0 {
		serverIDs := append([]string(nil), resources.ServerIDs...)
		floatingIPs := append([]string(nil), resources.FloatingIPs...)
		out.KiResources = &struct {
			FloatingIps *[]string `json:"floating_ips,omitempty"`
			NetworkId   *string   `json:"network_id,omitempty"`
			ServerIds   *[]string `json:"server_ids,omitempty"`
		}{
			FloatingIps: &floatingIPs,
			NetworkId:   optionalString(resources.NetworkID),
			ServerIds:   &serverIDs,
		}
	}
	if len(detail.DeploySteps) > 0 {
		steps := make([]LabDeployStep, 0, len(detail.DeploySteps))
		for _, step := range detail.DeploySteps {
			attempt := step.Attempt
			status := LabDeployStepStatus(step.Status)
			stepName := LabDeployStepStepName(step.StepName)
			lastError := step.LastError
			steps = append(steps, LabDeployStep{
				StepName:   &stepName,
				Status:     &status,
				Attempt:    &attempt,
				LastError:  &lastError,
				StartedAt:  step.StartedAt,
				FinishedAt: step.FinishedAt,
			})
		}
		out.DeploySteps = &steps
	}
	return out
}

type zeroizingReader struct {
	*bytes.Reader
	buf []byte
}

func newZeroizingReader(buf []byte) *zeroizingReader {
	return &zeroizingReader{Reader: bytes.NewReader(buf), buf: buf}
}

func (r *zeroizingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		for i := range r.buf {
			r.buf[i] = 0
		}
	}
	return n, err
}
