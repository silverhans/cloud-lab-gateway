package deploy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	labdomain "github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

// Persistent step statuses (subset of the lab_deploy_steps.status CHECK).
const (
	stepInProgress = "in_progress"
	stepSucceeded  = "succeeded"
	stepFailed     = "failed"
)

// TaskPayload is the JSON body of a TaskDeployLab (and TaskCleanupLab) task.
type TaskPayload struct {
	LabID string `json:"lab_id"`
}

// HandleTask is the asynq handler bound to ports.TaskDeployLab.
func (d Deps) HandleTask(ctx context.Context, t ports.Task) error {
	var p TaskPayload
	if err := json.Unmarshal(t.Payload, &p); err != nil {
		return fmt.Errorf("deploy: bad task payload: %w", err)
	}
	id, err := uuid.Parse(p.LabID)
	if err != nil {
		return fmt.Errorf("deploy: bad lab id %q: %w", p.LabID, err)
	}
	return d.Run(ctx, shared.LabInstanceID(id))
}

// Run executes the deploy saga for one lab.
//
// It is safe to call repeatedly: every step records its outcome in
// lab_deploy_steps, and a step already marked succeeded is skipped (its result
// is replayed into the in-memory state). So an asynq retry — or a brand-new
// worker after a crash — resumes from the first unfinished step rather than
// re-doing provider work.
//
// Failure handling per step:
//   - transient failure, attempts left  → return the error so asynq retries
//   - failure, retry budget exhausted   → compensate (lab → FAILED, hand
//     teardown to the cleanup saga) and return nil so asynq stops
func (d Deps) Run(ctx context.Context, labID shared.LabInstanceID) error {
	lab, err := d.Lab.GetByID(ctx, labID)
	if err != nil {
		return fmt.Errorf("deploy: load lab: %w", err)
	}
	if lab.State != labdomain.StateDeploying {
		// Cancelled, already deployed, or failed elsewhere. Returning nil
		// keeps asynq from retrying a task that has nothing to do.
		d.log().Info("deploy: lab not in deploying state, skipping",
			zap.String("lab_id", labID.String()),
			zap.String("state", string(lab.State)))
		return nil
	}
	if lab.ProjectID == nil {
		return fmt.Errorf("deploy: lab %s has no project assigned", labID)
	}

	st := &deployState{}
	for _, step := range orderedSteps() {
		rec, err := d.Steps.GetOrInit(ctx, labID, string(step.name))
		if err != nil {
			return fmt.Errorf("deploy: load step %s: %w", step.name, err)
		}

		if rec.Status == stepSucceeded {
			if len(rec.Result) > 0 {
				if err := json.Unmarshal(rec.Result, st); err != nil {
					return fmt.Errorf("deploy: replay step %s result: %w", step.name, err)
				}
			}
			continue
		}

		rec.Status = stepInProgress
		rec.Attempt++
		rec.LastError = ""
		started := d.now()
		rec.StartedAt = &started
		if err := d.Steps.Save(ctx, rec); err != nil {
			return fmt.Errorf("deploy: save step %s: %w", step.name, err)
		}

		stepErr := step.run(ctx, d, lab, st)
		finished := d.now()
		rec.FinishedAt = &finished

		if stepErr != nil {
			rec.Status = stepFailed
			rec.LastError = stepErr.Error()
			if err := d.Steps.Save(ctx, rec); err != nil {
				d.log().Error("deploy: save failed-step record failed", zap.Error(err))
			}
			if rec.Attempt >= d.maxAttempts() {
				d.log().Error("deploy: step exhausted retries — compensating",
					zap.String("lab_id", labID.String()),
					zap.String("step", string(step.name)),
					zap.Int("attempts", rec.Attempt),
					zap.Error(stepErr))
				return d.fail(ctx, lab, step.name, stepErr)
			}
			// Transient: let asynq retry the whole task. Succeeded steps are
			// skipped on the next pass, so no provider work is duplicated.
			return fmt.Errorf("deploy: step %s (attempt %d): %w", step.name, rec.Attempt, stepErr)
		}

		rec.Status = stepSucceeded
		result, mErr := json.Marshal(st)
		if mErr != nil {
			return fmt.Errorf("deploy: encode step %s result: %w", step.name, mErr)
		}
		rec.Result = result
		if err := d.Steps.Save(ctx, rec); err != nil {
			return fmt.Errorf("deploy: save step %s: %w", step.name, err)
		}
	}

	return d.succeed(ctx, lab, st)
}

// succeed records provider handles on the lab, moves it to READY, arms the
// auto-cleanup timer, and schedules the cleanup task.
func (d Deps) succeed(ctx context.Context, lab *labdomain.LabInstance, st *deployState) error {
	lab.KIResources = labdomain.KIResources{
		ServerIDs:   st.ServerIDs,
		NetworkID:   st.NetworkID,
		FloatingIPs: st.FloatingIPs,
		KeypairName: st.KeypairName,
	}
	if st.KeySecretID != "" {
		if id, err := uuid.Parse(st.KeySecretID); err == nil {
			sid := shared.SecretID(id)
			// One keypair serves both the checker and the student download.
			lab.CheckerSSHKeySecretID = &sid
			lab.StudentSSHKeySecretID = &sid
		}
	}

	now := d.now()
	cleanupAt := now.Add(d.cleanupAfter())
	if err := lab.MarkReady(cleanupAt, now); err != nil {
		return fmt.Errorf("deploy: mark ready: %w", err)
	}
	if err := d.saveLab(ctx, lab); err != nil {
		return err
	}

	payload, _ := json.Marshal(TaskPayload{LabID: lab.ID.String()})
	if _, err := d.Queue.EnqueueAt(ctx, ports.Task{
		Type:           ports.TaskCleanupLab,
		Payload:        payload,
		IdempotencyKey: "cleanup:" + lab.ID.String() + ":timer",
		MaxRetries:     5,
	}, cleanupAt); err != nil {
		// The lab is READY; a missed timer is recoverable by a reconciler
		// sweep over ListPendingCleanup. Surface it loudly meanwhile.
		d.log().Error("deploy: scheduling auto-cleanup failed",
			zap.String("lab_id", lab.ID.String()), zap.Error(err))
	}

	d.log().Info("deploy: lab ready", zap.String("lab_id", lab.ID.String()))
	return nil
}

// fail compensates an unrecoverable deploy: the lab moves to FAILED and the
// cleanup saga is enqueued to tear down whatever the partial deploy created.
// The cleanup saga is idempotent, so it copes with any subset of resources.
func (d Deps) fail(ctx context.Context, lab *labdomain.LabInstance, step stepName, cause error) error {
	now := d.now()
	reason := fmt.Sprintf("deploy failed at %s: %v", step, cause)
	if err := lab.Transition(labdomain.StateFailed, reason, now); err != nil {
		return fmt.Errorf("deploy: transition to failed: %w", err)
	}
	if err := d.saveLab(ctx, lab); err != nil {
		return err
	}

	payload, _ := json.Marshal(TaskPayload{LabID: lab.ID.String()})
	if _, err := d.Queue.Enqueue(ctx, ports.Task{
		Type:           ports.TaskCleanupLab,
		Payload:        payload,
		IdempotencyKey: "cleanup:" + lab.ID.String() + ":compensate",
		MaxRetries:     5,
	}); err != nil {
		d.log().Error("deploy: enqueue compensation cleanup failed",
			zap.String("lab_id", lab.ID.String()), zap.Error(err))
	}
	return nil
}

func (d Deps) saveLab(ctx context.Context, lab *labdomain.LabInstance) error {
	return d.UoW.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		return d.Lab.Save(ctx, tx, lab)
	})
}
