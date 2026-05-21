package httpapi

import (
	"context"
	"encoding/json"

	appadmin "github.com/cloud-lab-gateway/gateway/internal/app/admin"
	"github.com/cloud-lab-gateway/gateway/internal/domain/audit"
	"github.com/cloud-lab-gateway/gateway/internal/domain/pool"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (s *Server) GetAdminProjects(ctx context.Context, request GetAdminProjectsRequestObject) (GetAdminProjectsResponseObject, error) {
	if s.deps.Admin == nil {
		return nil, s.notImplemented()
	}
	principal, ok := principalFrom(ctx)
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	var state *pool.State
	if request.Params.State != nil {
		v := pool.State(*request.Params.State)
		state = &v
	}
	domain := ""
	if request.Params.KiDomainId != nil {
		domain = *request.Params.KiDomainId
	}
	projects, err := s.deps.Admin.ListProjects(ctx, adminActor(principal), domain, state)
	if err != nil {
		return nil, err
	}
	out := make([]Project, 0, len(projects))
	for i := range projects {
		out = append(out, projectResponse(&projects[i]))
	}
	return GetAdminProjects200JSONResponse(out), nil
}

func (s *Server) PostAdminProjectsIdQuarantine(ctx context.Context, request PostAdminProjectsIdQuarantineRequestObject) (PostAdminProjectsIdQuarantineResponseObject, error) {
	if s.deps.Admin == nil || request.Body == nil || request.Body.Reason == "" {
		return nil, shared.ErrInvalidInput
	}
	principal, ok := principalFrom(ctx)
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	if err := s.deps.Admin.QuarantineProject(ctx, adminActor(principal), shared.ProjectID(request.Id), request.Body.Reason); err != nil {
		return nil, err
	}
	return PostAdminProjectsIdQuarantine200Response{}, nil
}

func (s *Server) PostAdminProjectsIdRelease(ctx context.Context, request PostAdminProjectsIdReleaseRequestObject) (PostAdminProjectsIdReleaseResponseObject, error) {
	if s.deps.Admin == nil {
		return nil, s.notImplemented()
	}
	principal, ok := principalFrom(ctx)
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	if err := s.deps.Admin.ReleaseProject(ctx, adminActor(principal), shared.ProjectID(request.Id)); err != nil {
		return nil, err
	}
	return PostAdminProjectsIdRelease200Response{}, nil
}

func (s *Server) GetAdminQuota(ctx context.Context, _ GetAdminQuotaRequestObject) (GetAdminQuotaResponseObject, error) {
	if s.deps.Admin == nil {
		return nil, s.notImplemented()
	}
	principal, ok := principalFrom(ctx)
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	snap, err := s.deps.Admin.GetQuota(ctx, adminActor(principal))
	if err != nil {
		return nil, err
	}
	return GetAdminQuota200JSONResponse(quotaResponse(snap)), nil
}

func (s *Server) GetAdminSettings(ctx context.Context, request GetAdminSettingsRequestObject) (GetAdminSettingsResponseObject, error) {
	if s.deps.Admin == nil {
		return nil, s.notImplemented()
	}
	principal, ok := principalFrom(ctx)
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	scope := ""
	if request.Params.Scope != nil {
		scope = string(*request.Params.Scope)
	}
	var scopeID *string
	if request.Params.ScopeId != nil {
		v := request.Params.ScopeId.String()
		scopeID = &v
	}
	settings, err := s.deps.Admin.ListSettings(ctx, adminActor(principal), scope, scopeID)
	if err != nil {
		return nil, err
	}
	out := make([]Setting, 0, len(settings))
	for _, setting := range settings {
		out = append(out, settingResponse(setting))
	}
	return GetAdminSettings200JSONResponse(out), nil
}

func (s *Server) PutAdminSettings(ctx context.Context, request PutAdminSettingsRequestObject) (PutAdminSettingsResponseObject, error) {
	if s.deps.Admin == nil || request.Body == nil {
		return nil, shared.ErrInvalidInput
	}
	principal, ok := principalFrom(ctx)
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	value, err := json.Marshal(request.Body.Value)
	if err != nil {
		return nil, shared.ErrInvalidInput
	}
	var scopeID *string
	if request.Body.ScopeId != nil {
		v := request.Body.ScopeId.String()
		scopeID = &v
	}
	setting, err := s.deps.Admin.PutSetting(ctx, appadmin.PutSettingInput{
		Actor:   adminActor(principal),
		Key:     request.Body.Key,
		Value:   value,
		Scope:   string(request.Body.Scope),
		ScopeID: scopeID,
	})
	if err != nil {
		return nil, err
	}
	return PutAdminSettings200JSONResponse(settingResponse(setting)), nil
}

func (s *Server) GetAdminAudit(ctx context.Context, request GetAdminAuditRequestObject) (GetAdminAuditResponseObject, error) {
	if s.deps.Admin == nil {
		return nil, s.notImplemented()
	}
	principal, ok := principalFrom(ctx)
	if !ok {
		return nil, shared.ErrUnauthorized
	}
	var actorID *shared.UserID
	if request.Params.ActorUserId != nil {
		v := shared.UserID(*request.Params.ActorUserId)
		actorID = &v
	}
	events, err := s.deps.Admin.QueryAudit(ctx, adminActor(principal), ports.AuditFilter{
		Kind:        request.Params.Kind,
		ActorUserID: actorID,
		Since:       request.Params.Since,
		Limit:       intValue(request.Params.Limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]AuditEvent, 0, len(events))
	for _, ev := range events {
		out = append(out, auditResponse(ev))
	}
	return GetAdminAudit200JSONResponse(out), nil
}

func projectResponse(project *pool.Project) Project {
	id := openapi_types.UUID(project.ID)
	state := ProjectState(project.State)
	cleanupFailures := project.CleanupFailures
	out := Project{
		Id:                &id,
		KiProjectId:       &project.KIProjectID,
		KiDomainId:        &project.KIDomainID,
		Name:              &project.Name,
		State:             &state,
		CleanupFailures:   &cleanupFailures,
		LastStateChangeAt: timePtr(project.LastStateChangeAt),
	}
	if project.AllocatedToLabID != nil {
		v := openapi_types.UUID(*project.AllocatedToLabID)
		out.AllocatedToLabId = &v
	}
	return out
}

func quotaResponse(snap shared.QuotaSnapshot) QuotaSnapshot {
	vcpus := resourceResponse(snap.VCPUs)
	ram := resourceResponse(snap.RAM)
	disk := resourceResponse(snap.Disk)
	vcpusPct := float32(snap.VCPUs.UtilizationPct())
	ramPct := float32(snap.RAM.UtilizationPct())
	diskPct := float32(snap.Disk.UtilizationPct())
	maxPct := float32(snap.MaxUtilizationPct())
	return QuotaSnapshot{
		Vcpus:     &vcpus,
		Ram:       &ram,
		Disk:      &disk,
		FetchedAt: timePtr(snap.FetchedAt),
		UtilizationPct: &struct {
			Disk  *float32 `json:"disk,omitempty"`
			Max   *float32 `json:"max,omitempty"`
			Ram   *float32 `json:"ram,omitempty"`
			Vcpus *float32 `json:"vcpus,omitempty"`
		}{
			Vcpus: &vcpusPct,
			Ram:   &ramPct,
			Disk:  &diskPct,
			Max:   &maxPct,
		},
	}
}

func resourceResponse(cap shared.Capacity) Resource {
	used := float32(cap.Used)
	total := float32(cap.Total)
	unit := cap.Unit
	return Resource{Used: &used, Total: &total, Unit: &unit}
}

func settingResponse(setting ports.Setting) Setting {
	scope := SettingScope(setting.Scope)
	var value interface{} = map[string]interface{}{}
	if len(setting.Value) > 0 {
		_ = json.Unmarshal(setting.Value, &value)
	}
	out := Setting{
		Key:             &setting.Key,
		Value:           &value,
		Scope:           &scope,
		UpdatedAt:       timePtr(setting.UpdatedAt),
		UpdatedByUserId: uuidPtr(openapi_types.UUID(setting.UpdatedByUser)),
	}
	if setting.ScopeID != nil {
		if id, err := uuid.Parse(*setting.ScopeID); err == nil {
			typed := openapi_types.UUID(id)
			out.ScopeId = &typed
		}
	}
	return out
}

func auditResponse(ev audit.AuditEvent) AuditEvent {
	payload := ev.Payload
	out := AuditEvent{
		Id:          uuidPtr(openapi_types.UUID(ev.ID)),
		Kind:        &ev.Kind,
		SubjectType: &ev.SubjectType,
		Payload:     &payload,
		RequestId:   optionalString(ev.RequestID),
		OccurredAt:  timePtr(ev.OccurredAt),
	}
	if ev.ActorUserID != nil {
		out.ActorUserId = uuidPtr(openapi_types.UUID(*ev.ActorUserID))
	}
	if ev.SubjectID != nil {
		if id, err := uuid.Parse(*ev.SubjectID); err == nil {
			typed := openapi_types.UUID(id)
			out.SubjectId = &typed
		}
	}
	return out
}

func intValue(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
