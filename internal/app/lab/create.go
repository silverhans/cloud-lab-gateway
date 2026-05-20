package lab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-lab-gateway/gateway/internal/domain/audit"
	labdomain "github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	"github.com/cloud-lab-gateway/gateway/internal/domain/quota"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

// defaultQuotaMaxAge bounds how stale a quota snapshot may be before CreateLab
// refuses to decide on it.
const defaultQuotaMaxAge = 60 * time.Second

// CreateInput is the request to provision a lab for a student.
type CreateInput struct {
	StudentUserID shared.UserID
	CourseID      shared.CourseID
	LabTemplateID shared.LabTemplateID
	RequestID     string // correlation id from the inbound HTTP/LTI request
}

// DeployTaskPayload is the JSON payload of a TaskDeployLab task. The deploy
// saga decodes it.
type DeployTaskPayload struct {
	LabID string `json:"lab_id"`
}

// CreateLab is the critical-path use case: validate cluster capacity, rent a
// project from the pool, persist the lab aggregate, and hand the deploy off to
// the async worker. The sequence below is deliberate — see docs/STATE_MACHINES
// §5.4 — and must not be reordered.
func (d Deps) CreateLab(ctx context.Context, in CreateInput) (*labdomain.LabInstance, error) {
	now := d.Clock.Now()
	req := resourceRequestFor(in.LabTemplateID)

	threshold := d.QuotaThresholdPct
	if threshold <= 0 {
		threshold = quota.DefaultThresholdPct
	}

	// --- Capacity guard (criterion 2) ------------------------------------
	snap, age, err := d.QuotaCache.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("create lab: read quota cache: %w", err)
	}
	maxAge := d.QuotaMaxAge
	if maxAge <= 0 {
		maxAge = defaultQuotaMaxAge
	}
	if age > maxAge {
		// Stale snapshot: refresh in the background and fail closed. Deciding
		// capacity on outdated data risks overcommitting the cluster.
		if _, qErr := d.Queue.Enqueue(ctx, ports.Task{Type: ports.TaskRefreshQuota}); qErr != nil {
			d.log().Warn("quota refresh enqueue failed", zap.Error(qErr))
		}
		return nil, fmt.Errorf("create lab: quota snapshot stale (age %s): %w", age, shared.ErrCloudUnavailable)
	}

	decision := quota.Evaluate(snap, req, threshold)
	if !decision.Allowed {
		d.appendAudit(ctx, nil, audit.AuditEvent{
			ID:          shared.NewAuditEventID(),
			Kind:        audit.KindQuotaBlocked,
			ActorUserID: &in.StudentUserID,
			SubjectType: "lab_template",
			SubjectID:   strptr(in.LabTemplateID.String()),
			Payload: map[string]any{
				"reason":        decision.Reason,
				"predicted_pct": decision.PredictedPct,
				"threshold_pct": decision.ThresholdPct,
			},
			RequestID:  in.RequestID,
			OccurredAt: now,
		})
		return nil, shared.ErrQuotaExceeded
	}

	// --- Allocate + persist atomically -----------------------------------
	var created *labdomain.LabInstance
	var poolEmpty bool

	txErr := d.UoW.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		course, err := d.Courses.GetByID(ctx, in.CourseID)
		if err != nil {
			return fmt.Errorf("resolve course: %w", err)
		}

		active, err := d.Lab.FindActiveByStudentAndCourse(ctx, in.StudentUserID, in.CourseID)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return fmt.Errorf("check active lab: %w", err)
		}
		if active != nil {
			return shared.ErrLabAlreadyActive
		}

		inst := labdomain.New(shared.NewLabInstanceID(), in.StudentUserID, in.CourseID, in.LabTemplateID, now)
		if err := inst.Transition(labdomain.StatePendingProject, "quota_ok", now); err != nil {
			return err
		}
		if err := d.Lab.Create(ctx, tx, inst); err != nil {
			return fmt.Errorf("persist lab: %w", err)
		}

		proj, allocErr := d.Pool.AllocateOneFree(ctx, tx, course.KIDomainID, inst.ID)
		if errors.Is(allocErr, shared.ErrPoolEmpty) {
			// Record the rejection as history: the lab row is committed in
			// `rejected` state so the student and teacher can see the attempt.
			if err := inst.Transition(labdomain.StateRejected, "pool_empty", now); err != nil {
				return err
			}
			if err := d.Lab.Save(ctx, tx, inst); err != nil {
				return fmt.Errorf("save rejected lab: %w", err)
			}
			d.appendAuditTx(ctx, tx, audit.AuditEvent{
				ID:          shared.NewAuditEventID(),
				Kind:        audit.KindQuotaBlocked,
				ActorUserID: &in.StudentUserID,
				SubjectType: "lab_instance",
				SubjectID:   strptr(inst.ID.String()),
				Payload:     map[string]any{"reason": "pool_empty", "ki_domain_id": course.KIDomainID},
				RequestID:   in.RequestID,
				OccurredAt:  now,
			})
			poolEmpty = true
			return nil // commit the rejected lab
		}
		if allocErr != nil {
			return fmt.Errorf("allocate project: %w", allocErr)
		}

		if err := inst.AssignProject(proj.ID, now); err != nil {
			return err
		}
		if err := d.Lab.Save(ctx, tx, inst); err != nil {
			return fmt.Errorf("save lab: %w", err)
		}
		if err := d.Pool.Save(ctx, tx, proj); err != nil {
			return fmt.Errorf("save project: %w", err)
		}
		created = inst
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}
	if poolEmpty {
		return nil, shared.ErrPoolEmpty
	}

	// --- Hand off to the async deploy saga -------------------------------
	payload, _ := json.Marshal(DeployTaskPayload{LabID: created.ID.String()})
	if _, err := d.Queue.Enqueue(ctx, ports.Task{
		Type:           ports.TaskDeployLab,
		Payload:        payload,
		IdempotencyKey: "deploy:" + created.ID.String() + ":1",
		MaxRetries:     5,
	}); err != nil {
		// The lab is committed in `deploying` state. A missed enqueue is
		// recoverable by a reconciler sweep; surface it loudly meanwhile.
		d.log().Error("deploy task enqueue failed — lab stuck in deploying",
			zap.String("lab_id", created.ID.String()),
			zap.Error(err),
		)
	}
	return created, nil
}

// resourceRequestFor returns the resources a lab template needs.
//
// TODO: read this from the persisted lab_templates.topology JSONB once the
// LabTemplate repository exists. For now every template gets the same modest
// footprint, which is enough for the quota guard to be meaningful.
func resourceRequestFor(_ shared.LabTemplateID) shared.ResourceRequest {
	return shared.ResourceRequest{VCPUs: 2, RAMMB: 4096, DiskGB: 20}
}

func (d Deps) appendAudit(ctx context.Context, _ ports.Tx, ev audit.AuditEvent) {
	if err := d.Audit.Append(ctx, ev); err != nil {
		d.log().Warn("append audit event failed", zap.String("kind", ev.Kind), zap.Error(err))
	}
}

func (d Deps) appendAuditTx(ctx context.Context, tx ports.Tx, ev audit.AuditEvent) {
	if err := d.Audit.AppendInTx(ctx, tx, ev); err != nil {
		d.log().Warn("append audit event in tx failed", zap.String("kind", ev.Kind), zap.Error(err))
	}
}

func strptr(s string) *string { return &s }
