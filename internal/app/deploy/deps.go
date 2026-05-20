// Package deploy implements the deploy saga: the async workflow that turns a
// freshly-allocated lab (state `deploying`) into a running, reachable stand
// (state `ready`). See docs/STATE_MACHINES.md §4.
package deploy

import (
	"time"

	"go.uber.org/zap"

	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

// defaultMaxAttempts is the per-step retry budget before the saga gives up and
// hands the lab to the cleanup saga for compensation.
const defaultMaxAttempts = 3

// Deps bundles the collaborators the deploy saga needs.
type Deps struct {
	Cloud   ports.CloudProvider
	Lab     ports.LabRepo
	Steps   ports.DeployStepRepo
	Secrets ports.SecretStore
	Queue   ports.TaskQueue
	UoW     ports.UnitOfWork
	Clock   ports.Clock
	Logger  *zap.Logger

	// Checks runs the post-deploy initial check. Optional — when nil the
	// initial-check step is recorded as skipped (it is informational only).
	Checks ports.CheckRunner

	// CleanupAfter is how long a ready lab lives before auto-cleanup.
	CleanupAfter time.Duration

	// MaxAttempts caps per-step retries. 0 → defaultMaxAttempts.
	MaxAttempts int
}

func (d Deps) log() *zap.Logger {
	if d.Logger == nil {
		return zap.NewNop()
	}
	return d.Logger
}

func (d Deps) now() time.Time { return d.Clock.Now() }

func (d Deps) maxAttempts() int {
	if d.MaxAttempts > 0 {
		return d.MaxAttempts
	}
	return defaultMaxAttempts
}

func (d Deps) cleanupAfter() time.Duration {
	if d.CleanupAfter > 0 {
		return d.CleanupAfter
	}
	return 2 * time.Hour
}
