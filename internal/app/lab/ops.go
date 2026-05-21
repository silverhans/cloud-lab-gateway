package lab

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/identity"
	labdomain "github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"go.uber.org/zap"
)

const (
	defaultListLimit       = 200
	defaultCleanupDuration = 2 * time.Hour
	defaultFreezeDuration  = 24 * time.Hour
)

type StopLabInput struct {
	Actor     Actor
	LabID     shared.LabInstanceID
	RequestID string
}

type FreezeLabInput struct {
	Actor            Actor
	LabID            shared.LabInstanceID
	Reason           string
	FreezeForSeconds *int
}

type UnfreezeLabInput struct {
	Actor Actor
	LabID shared.LabInstanceID
}

type ExtendLabInput struct {
	Actor    Actor
	LabID    shared.LabInstanceID
	ExtendBy time.Duration
}

func (d Deps) ListLabs(ctx context.Context, actor Actor, courseID *shared.CourseID, states []labdomain.State) ([]labdomain.LabInstance, error) {
	reader, err := d.reader()
	if err != nil {
		return nil, err
	}
	filter := LabListFilter{CourseID: courseID, States: states, Limit: defaultListLimit}
	switch actor.Role {
	case identity.RoleAdmin:
	case identity.RoleTeacher:
		if courseID != nil {
			if actor.CourseRoles[*courseID] != identity.CourseRoleTeacher {
				return nil, shared.ErrForbidden
			}
		} else {
			filter.CourseIDs = taughtCourseIDs(actor)
			if len(filter.CourseIDs) == 0 {
				return []labdomain.LabInstance{}, nil
			}
		}
	default:
		filter.StudentUserID = &actor.UserID
	}
	return reader.List(ctx, filter)
}

func (d Deps) GetLab(ctx context.Context, actor Actor, id shared.LabInstanceID) (*LabDetail, error) {
	lab, err := d.Lab.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := canLab(ctx, actor, identity.ActionLabRead, lab); err != nil {
		return nil, err
	}
	var steps []ports.DeployStep
	if d.Steps != nil {
		steps, err = d.Steps.ListByLab(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("get lab: deploy steps: %w", err)
		}
	}
	return &LabDetail{Lab: lab, DeploySteps: steps}, nil
}

func (d Deps) StopLab(ctx context.Context, in StopLabInput) (*labdomain.LabInstance, error) {
	lab, err := d.Lab.GetByID(ctx, in.LabID)
	if err != nil {
		return nil, err
	}
	if err := canLab(ctx, in.Actor, identity.ActionLabDelete, lab); err != nil {
		return nil, err
	}
	if lab.State != labdomain.StateReady && lab.State != labdomain.StateFrozen && lab.State != labdomain.StateFailed {
		return nil, shared.ErrInvalidTransition{Entity: "lab", From: string(lab.State), To: string(labdomain.StateCleaning)}
	}
	now := d.now()
	if err := lab.Transition(labdomain.StateCleaning, "stop_requested", now); err != nil {
		return nil, err
	}
	if err := d.saveLab(ctx, lab); err != nil {
		return nil, err
	}
	if d.Queue != nil {
		payload, _ := json.Marshal(DeployTaskPayload{LabID: lab.ID.String()})
		if _, err := d.Queue.Enqueue(ctx, ports.Task{
			Type:           ports.TaskCleanupLab,
			Payload:        payload,
			IdempotencyKey: "cleanup:" + lab.ID.String() + ":manual",
			MaxRetries:     5,
		}); err != nil {
			d.log().Error("cleanup task enqueue failed",
				zap.String("lab_id", lab.ID.String()),
				zap.Error(err),
			)
		}
	}
	return lab, nil
}

func (d Deps) FreezeLab(ctx context.Context, in FreezeLabInput) (*labdomain.LabInstance, error) {
	lab, err := d.Lab.GetByID(ctx, in.LabID)
	if err != nil {
		return nil, err
	}
	if err := canLab(ctx, in.Actor, identity.ActionLabFreeze, lab); err != nil {
		return nil, err
	}
	if in.FreezeForSeconds != nil && in.Actor.Role == identity.RoleStudent {
		return nil, shared.ErrForbidden
	}
	duration := d.freezeFor()
	if in.FreezeForSeconds != nil {
		duration = time.Duration(*in.FreezeForSeconds) * time.Second
	}
	now := d.now()
	if err := lab.Freeze(in.Actor.UserID, in.Reason, now.Add(duration), now); err != nil {
		return nil, err
	}
	if err := d.saveLab(ctx, lab); err != nil {
		return nil, err
	}
	return lab, nil
}

func (d Deps) UnfreezeLab(ctx context.Context, in UnfreezeLabInput) (*labdomain.LabInstance, error) {
	lab, err := d.Lab.GetByID(ctx, in.LabID)
	if err != nil {
		return nil, err
	}
	if err := canLab(ctx, in.Actor, identity.ActionLabUnfreeze, lab); err != nil {
		return nil, err
	}
	now := d.now()
	if err := lab.Unfreeze(now.Add(d.cleanupAfter()), now); err != nil {
		return nil, err
	}
	if err := d.saveLab(ctx, lab); err != nil {
		return nil, err
	}
	return lab, nil
}

func (d Deps) ExtendLab(ctx context.Context, in ExtendLabInput) (*labdomain.LabInstance, error) {
	lab, err := d.Lab.GetByID(ctx, in.LabID)
	if err != nil {
		return nil, err
	}
	if err := canLab(ctx, in.Actor, identity.ActionLabExtend, lab); err != nil {
		return nil, err
	}
	now := d.now()
	if err := lab.ExtendCleanup(now.Add(in.ExtendBy), now); err != nil {
		return nil, err
	}
	if err := d.saveLab(ctx, lab); err != nil {
		return nil, err
	}
	return lab, nil
}

func (d Deps) GetLabSSHKey(ctx context.Context, actor Actor, id shared.LabInstanceID) ([]byte, error) {
	lab, err := d.Lab.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if lab.StudentUserID != actor.UserID {
		return nil, shared.ErrForbidden
	}
	if lab.StudentSSHKeySecretID == nil || d.Secrets == nil {
		return nil, shared.ErrNotFound
	}
	key, err := d.Secrets.Get(ctx, *lab.StudentSSHKeySecretID, "ssh_private_key", lab.ID.String())
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (d Deps) reader() (LabReader, error) {
	if d.LabReader != nil {
		return d.LabReader, nil
	}
	if reader, ok := d.Lab.(LabReader); ok {
		return reader, nil
	}
	return nil, shared.ErrInvalidInput
}

func (d Deps) saveLab(ctx context.Context, lab *labdomain.LabInstance) error {
	return d.UoW.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		return d.Lab.Save(ctx, tx, lab)
	})
}

func (d Deps) now() time.Time {
	if d.Clock == nil {
		return time.Now().UTC()
	}
	return d.Clock.Now()
}

func (d Deps) cleanupAfter() time.Duration {
	if d.DefaultCleanupAfter > 0 {
		return d.DefaultCleanupAfter
	}
	return defaultCleanupDuration
}

func (d Deps) freezeFor() time.Duration {
	if d.DefaultFreezeFor > 0 {
		return d.DefaultFreezeFor
	}
	return defaultFreezeDuration
}

func canLab(ctx context.Context, actor Actor, action identity.Action, lab *labdomain.LabInstance) error {
	return identity.DefaultPolicy{}.Can(ctx, identity.Subject{
		UserID:      actor.UserID,
		Role:        actor.Role,
		CourseRoles: actor.CourseRoles,
	}, action, identity.LabResource{
		LabID:    lab.ID,
		OwnerID:  lab.StudentUserID,
		CourseID: lab.CourseID,
	})
}

func taughtCourseIDs(actor Actor) []shared.CourseID {
	out := make([]shared.CourseID, 0, len(actor.CourseRoles))
	for id, role := range actor.CourseRoles {
		if role == identity.CourseRoleTeacher {
			out = append(out, id)
		}
	}
	return out
}
