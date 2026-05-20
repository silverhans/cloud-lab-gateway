package httpapi

import (
	"context"
	"errors"
)

var errNotImplemented = errors.New("not implemented")

func (s *Server) notImplemented() error { return errNotImplemented }

func (s *Server) GetAdminAudit(context.Context, GetAdminAuditRequestObject) (GetAdminAuditResponseObject, error) {
	return nil, s.notImplemented()
}

func (s *Server) GetAdminProjects(context.Context, GetAdminProjectsRequestObject) (GetAdminProjectsResponseObject, error) {
	return nil, s.notImplemented()
}

func (s *Server) PostAdminProjectsIdQuarantine(context.Context, PostAdminProjectsIdQuarantineRequestObject) (PostAdminProjectsIdQuarantineResponseObject, error) {
	return nil, s.notImplemented()
}

func (s *Server) PostAdminProjectsIdRelease(context.Context, PostAdminProjectsIdReleaseRequestObject) (PostAdminProjectsIdReleaseResponseObject, error) {
	return nil, s.notImplemented()
}

func (s *Server) GetAdminQuota(context.Context, GetAdminQuotaRequestObject) (GetAdminQuotaResponseObject, error) {
	return nil, s.notImplemented()
}

func (s *Server) GetAdminSettings(context.Context, GetAdminSettingsRequestObject) (GetAdminSettingsResponseObject, error) {
	return nil, s.notImplemented()
}

func (s *Server) PutAdminSettings(context.Context, PutAdminSettingsRequestObject) (PutAdminSettingsResponseObject, error) {
	return nil, s.notImplemented()
}

func (s *Server) PostAuthLogin(context.Context, PostAuthLoginRequestObject) (PostAuthLoginResponseObject, error) {
	return nil, s.notImplemented()
}

func (s *Server) GetChecksId(context.Context, GetChecksIdRequestObject) (GetChecksIdResponseObject, error) {
	return nil, s.notImplemented()
}

func (s *Server) GetLabs(context.Context, GetLabsRequestObject) (GetLabsResponseObject, error) {
	return nil, s.notImplemented()
}

func (s *Server) DeleteLabsId(context.Context, DeleteLabsIdRequestObject) (DeleteLabsIdResponseObject, error) {
	return nil, s.notImplemented()
}

func (s *Server) GetLabsId(context.Context, GetLabsIdRequestObject) (GetLabsIdResponseObject, error) {
	return nil, s.notImplemented()
}

func (s *Server) GetLabsIdChecks(context.Context, GetLabsIdChecksRequestObject) (GetLabsIdChecksResponseObject, error) {
	return nil, s.notImplemented()
}

func (s *Server) PostLabsIdChecks(context.Context, PostLabsIdChecksRequestObject) (PostLabsIdChecksResponseObject, error) {
	return nil, s.notImplemented()
}

func (s *Server) PostLabsIdExtend(context.Context, PostLabsIdExtendRequestObject) (PostLabsIdExtendResponseObject, error) {
	return nil, s.notImplemented()
}

func (s *Server) PostLabsIdFreeze(context.Context, PostLabsIdFreezeRequestObject) (PostLabsIdFreezeResponseObject, error) {
	return nil, s.notImplemented()
}

func (s *Server) GetLabsIdSshKey(context.Context, GetLabsIdSshKeyRequestObject) (GetLabsIdSshKeyResponseObject, error) {
	return nil, s.notImplemented()
}

func (s *Server) PostLabsIdUnfreeze(context.Context, PostLabsIdUnfreezeRequestObject) (PostLabsIdUnfreezeResponseObject, error) {
	return nil, s.notImplemented()
}

func (s *Server) GetStreamLabs(context.Context, GetStreamLabsRequestObject) (GetStreamLabsResponseObject, error) {
	return nil, s.notImplemented()
}
