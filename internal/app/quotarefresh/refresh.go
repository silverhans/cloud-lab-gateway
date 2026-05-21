package quotarefresh

import (
	"context"
	"fmt"

	applab "github.com/cloud-lab-gateway/gateway/internal/app/lab"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"go.uber.org/zap"
)

// Deps refreshes the cached cluster quota snapshot from the cloud provider.
type Deps struct {
	Cloud      ports.CloudProvider
	QuotaCache applab.QuotaCache
	Logger     *zap.Logger
}

func (d Deps) HandleTask(ctx context.Context, _ ports.Task) error {
	if d.Cloud == nil || d.QuotaCache == nil {
		return fmt.Errorf("quotarefresh: missing dependencies")
	}
	snap, err := d.Cloud.GetQuotaSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("quotarefresh: get quota snapshot: %w", err)
	}
	if err := d.QuotaCache.Write(ctx, snap); err != nil {
		return fmt.Errorf("quotarefresh: write quota cache: %w", err)
	}
	d.log().Info("quota snapshot refreshed")
	return nil
}

func (d Deps) log() *zap.Logger {
	if d.Logger == nil {
		return zap.NewNop()
	}
	return d.Logger
}
