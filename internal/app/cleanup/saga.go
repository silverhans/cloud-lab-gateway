// Package cleanup implements the cleanup saga: tearing down a lab's КИ
// resources and returning its project to the pool. It is triggered by the
// auto-cleanup timer, an explicit student/teacher stop, an expired freeze, or
// as the compensation path of a failed deploy. See docs/STATE_MACHINES.md §5.
package cleanup

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	labdomain "github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	pooldomain "github.com/cloud-lab-gateway/gateway/internal/domain/pool"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

// Deps bundles the collaborators the cleanup saga needs.
type Deps struct {
	Cloud   ports.CloudProvider
	Lab     ports.LabRepo
	Pool    ports.PoolRepo
	Secrets ports.SecretStore
	UoW     ports.UnitOfWork
	Clock   ports.Clock
	Logger  *zap.Logger
}

// TaskPayload is the JSON body of a TaskCleanupLab task.
type TaskPayload struct {
	LabID string `json:"lab_id"`
}

func (d Deps) log() *zap.Logger {
	if d.Logger == nil {
		return zap.NewNop()
	}
	return d.Logger
}

// HandleTask is the asynq handler bound to ports.TaskCleanupLab.
func (d Deps) HandleTask(ctx context.Context, t ports.Task) error {
	var p TaskPayload
	if err := json.Unmarshal(t.Payload, &p); err != nil {
		return fmt.Errorf("cleanup: bad task payload: %w", err)
	}
	id, err := uuid.Parse(p.LabID)
	if err != nil {
		return fmt.Errorf("cleanup: bad lab id %q: %w", p.LabID, err)
	}
	return d.Run(ctx, shared.LabInstanceID(id))
}

// Run tears down a lab. It is idempotent: provider deletes treat "not found"
// as success, so a retry after a partial teardown simply finishes the job.
//
// Failure handling: each failed teardown increments the project's
// cleanup-failure counter. After pool.MaxCleanupFailures the project is
// quarantined for manual review and the saga stops retrying — the lab is done
// as far as the student is concerned, the operational debt is the project's.
func (d Deps) Run(ctx context.Context, labID shared.LabInstanceID) error {
	now := d.Clock.Now()

	lab, err := d.Lab.GetByID(ctx, labID)
	if err != nil {
		return fmt.Errorf("cleanup: load lab: %w", err)
	}
	if lab.State == labdomain.StateDone {
		return nil // already cleaned up
	}

	// Move the lab into `cleaning` (idempotent — skip if already there).
	if lab.State != labdomain.StateCleaning {
		if !labdomain.CanTransition(lab.State, labdomain.StateCleaning) {
			// Pending/deploying labs have nothing provisioned to tear down;
			// a stray cleanup task for them is a no-op, not an error.
			d.log().Warn("cleanup: lab not in a cleanable state, skipping",
				zap.String("lab_id", labID.String()),
				zap.String("state", string(lab.State)))
			return nil
		}
		if err := lab.Transition(labdomain.StateCleaning, "cleanup", now); err != nil {
			return fmt.Errorf("cleanup: transition to cleaning: %w", err)
		}
		if err := d.saveLab(ctx, lab); err != nil {
			return err
		}
	}

	// Load the project and move it to `cleaning` if still allocated.
	var project *pooldomain.Project
	if lab.ProjectID != nil {
		project, err = d.Pool.GetByID(ctx, *lab.ProjectID)
		if err != nil {
			return fmt.Errorf("cleanup: load project: %w", err)
		}
		if project.State == pooldomain.StateAllocated {
			if err := project.Release(now); err != nil {
				return fmt.Errorf("cleanup: release project: %w", err)
			}
			if err := d.saveProject(ctx, project); err != nil {
				return err
			}
		}
	}

	// Tear down the КИ resources.
	if tdErr := d.teardown(ctx, lab); tdErr != nil {
		if project != nil && project.State == pooldomain.StateCleaning {
			if err := project.RecordCleanupFailure(tdErr.Error(), now); err != nil {
				d.log().Error("cleanup: record failure", zap.Error(err))
			}
			if err := d.saveProject(ctx, project); err != nil {
				return err
			}
			if project.State == pooldomain.StateQuarantine {
				// Retry budget exhausted — quarantine and stop. The lab is
				// marked done; the project waits for an admin.
				d.log().Error("cleanup: project quarantined after repeated failures",
					zap.String("project_id", project.ID.String()),
					zap.Int("failures", project.CleanupFailures))
				return d.finishLab(ctx, lab, "cleanup_failed_quarantined", now)
			}
		}
		return tdErr // asynq retries
	}

	// Teardown succeeded — return the project to the pool.
	if project != nil && project.State == pooldomain.StateCleaning {
		if err := project.MarkClean(now); err != nil {
			return fmt.Errorf("cleanup: mark project clean: %w", err)
		}
		if err := d.saveProject(ctx, project); err != nil {
			return err
		}
	}

	// Drop the stored SSH key material.
	if lab.CheckerSSHKeySecretID != nil {
		if err := d.Secrets.Delete(ctx, *lab.CheckerSSHKeySecretID); err != nil {
			d.log().Warn("cleanup: delete ssh secret failed", zap.Error(err))
		}
	}

	return d.finishLab(ctx, lab, "cleanup_complete", now)
}

// teardown deletes every КИ resource the lab holds. Order: servers, keypair,
// network. Every provider delete is idempotent (not-found is success).
func (d Deps) teardown(ctx context.Context, lab *labdomain.LabInstance) error {
	if lab.ProjectID == nil {
		return nil
	}
	projectID := lab.ProjectID.String()
	res := lab.KIResources

	for _, serverID := range res.ServerIDs {
		if err := d.Cloud.DeleteServer(ctx, projectID, serverID); err != nil {
			return fmt.Errorf("delete server %s: %w", serverID, err)
		}
	}
	if res.KeypairName != "" {
		if err := d.Cloud.DeleteKeypair(ctx, projectID, res.KeypairName); err != nil {
			return fmt.Errorf("delete keypair %s: %w", res.KeypairName, err)
		}
	}
	if res.NetworkID != "" {
		if err := d.Cloud.DeleteNetwork(ctx, projectID, res.NetworkID); err != nil {
			return fmt.Errorf("delete network %s: %w", res.NetworkID, err)
		}
	}
	return nil
}

// finishLab is the saga's single terminal transition: lab → DONE.
func (d Deps) finishLab(ctx context.Context, lab *labdomain.LabInstance, reason string, now time.Time) error {
	if err := lab.Transition(labdomain.StateDone, reason, now); err != nil {
		return fmt.Errorf("cleanup: transition to done: %w", err)
	}
	if err := d.saveLab(ctx, lab); err != nil {
		return err
	}
	d.log().Info("cleanup: lab done",
		zap.String("lab_id", lab.ID.String()),
		zap.String("reason", reason))
	return nil
}

func (d Deps) saveLab(ctx context.Context, lab *labdomain.LabInstance) error {
	return d.UoW.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		return d.Lab.Save(ctx, tx, lab)
	})
}

func (d Deps) saveProject(ctx context.Context, p *pooldomain.Project) error {
	return d.UoW.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		return d.Pool.Save(ctx, tx, p)
	})
}
