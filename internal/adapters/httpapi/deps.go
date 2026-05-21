package httpapi

import (
	"context"

	applab "github.com/cloud-lab-gateway/gateway/internal/app/lab"
	appmoodle "github.com/cloud-lab-gateway/gateway/internal/app/moodle"
	labdomain "github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	"go.uber.org/zap"
)

// LabService is the narrow use-case seam used by HTTP handlers.
type LabService interface {
	CreateLab(ctx context.Context, in applab.CreateInput) (*labdomain.LabInstance, error)
}

// MoodleLaunchService is the app-layer seam for browser LTI launches.
type MoodleLaunchService interface {
	HandleLaunch(ctx context.Context, in appmoodle.LaunchInput) (*appmoodle.LaunchResult, error)
}

// Deps bundles HTTP handler collaborators.
type Deps struct {
	Lab           LabService
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
