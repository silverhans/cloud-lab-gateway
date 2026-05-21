package httpapi

import (
	"context"

	appadmin "github.com/cloud-lab-gateway/gateway/internal/app/admin"
	applab "github.com/cloud-lab-gateway/gateway/internal/app/lab"
	appmoodle "github.com/cloud-lab-gateway/gateway/internal/app/moodle"
	appverify "github.com/cloud-lab-gateway/gateway/internal/app/verify"
	"github.com/cloud-lab-gateway/gateway/internal/domain/audit"
	labdomain "github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	"github.com/cloud-lab-gateway/gateway/internal/domain/pool"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/domain/verify"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"go.uber.org/zap"
)

// LabService is the narrow use-case seam used by HTTP handlers.
type LabService interface {
	CreateLab(ctx context.Context, in applab.CreateInput) (*labdomain.LabInstance, error)
}

type LabOpsService interface {
	ListLabs(ctx context.Context, actor applab.Actor, courseID *shared.CourseID, states []labdomain.State) ([]labdomain.LabInstance, error)
	GetLab(ctx context.Context, actor applab.Actor, id shared.LabInstanceID) (*applab.LabDetail, error)
	StopLab(ctx context.Context, in applab.StopLabInput) (*labdomain.LabInstance, error)
	FreezeLab(ctx context.Context, in applab.FreezeLabInput) (*labdomain.LabInstance, error)
	UnfreezeLab(ctx context.Context, in applab.UnfreezeLabInput) (*labdomain.LabInstance, error)
	ExtendLab(ctx context.Context, in applab.ExtendLabInput) (*labdomain.LabInstance, error)
	GetLabSSHKey(ctx context.Context, actor applab.Actor, id shared.LabInstanceID) ([]byte, error)
}

type VerifyService interface {
	RunCheck(ctx context.Context, actor appverify.Actor, labID shared.LabInstanceID, templateID shared.CheckTemplate) (*verify.CheckRun, error)
	GetCheckRun(ctx context.Context, actor appverify.Actor, id shared.CheckRunID) (*verify.CheckRun, error)
	ListChecksByLab(ctx context.Context, actor appverify.Actor, labID shared.LabInstanceID) ([]verify.CheckRun, error)
}

type AdminService interface {
	ListProjects(ctx context.Context, actor appadmin.Actor, kiDomainID string, state *pool.State) ([]pool.Project, error)
	QuarantineProject(ctx context.Context, actor appadmin.Actor, id shared.ProjectID, reason string) error
	ReleaseProject(ctx context.Context, actor appadmin.Actor, id shared.ProjectID) error
	GetQuota(ctx context.Context, actor appadmin.Actor) (shared.QuotaSnapshot, error)
	ListSettings(ctx context.Context, actor appadmin.Actor, scope string, scopeID *string) ([]ports.Setting, error)
	PutSetting(ctx context.Context, in appadmin.PutSettingInput) (ports.Setting, error)
	QueryAudit(ctx context.Context, actor appadmin.Actor, f ports.AuditFilter) ([]audit.AuditEvent, error)
}

// MoodleLaunchService is the app-layer seam for browser LTI launches.
type MoodleLaunchService interface {
	HandleLaunch(ctx context.Context, in appmoodle.LaunchInput) (*appmoodle.LaunchResult, error)
}

// Deps bundles HTTP handler collaborators.
type Deps struct {
	Lab           LabService
	LabOps        LabOpsService
	Verify        VerifyService
	Admin         AdminService
	Logger        *zap.Logger
	DevMode       bool
	SessionSecret string
}

func (d Deps) log() *zap.Logger {
	if d.Logger == nil {
		return zap.NewNop()
	}
	return d.Logger
}
