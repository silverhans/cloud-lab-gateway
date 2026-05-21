package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/identity"
	labdomain "github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	verifydomain "github.com/cloud-lab-gateway/gateway/internal/domain/verify"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const defaultSSHUser = "ubuntu"

type Actor struct {
	UserID      shared.UserID
	Role        identity.Role
	CourseRoles map[shared.CourseID]identity.CourseRole
}

type Deps struct {
	UoW       ports.UnitOfWork
	Labs      ports.LabRepo
	Runs      ports.CheckRunRepo
	Templates ports.CheckTemplateRepo
	Secrets   ports.SecretStore
	Queue     ports.TaskQueue
	Runner    ports.CheckRunner
	Clock     ports.Clock
	Logger    *zap.Logger
}

type TaskPayload struct {
	CheckRunID string `json:"check_run_id"`
}

func (d Deps) RunCheck(ctx context.Context, actor Actor, labID shared.LabInstanceID, templateID shared.CheckTemplate) (*verifydomain.CheckRun, error) {
	lab, err := d.Labs.GetByID(ctx, labID)
	if err != nil {
		return nil, err
	}
	if err := canLab(ctx, actor, identity.ActionCheckRun, lab); err != nil {
		return nil, err
	}
	if lab.State != labdomain.StateReady {
		return nil, shared.ErrInvalidTransition{Entity: "lab", From: string(lab.State), To: "run_check"}
	}
	if d.Templates != nil {
		if _, err := d.Templates.GetByID(ctx, templateID); err != nil {
			return nil, err
		}
	}

	triggeredBy := actor.UserID
	run := &verifydomain.CheckRun{
		ID:              shared.NewCheckRunID(),
		LabInstanceID:   lab.ID,
		CheckTemplateID: templateID,
		TriggeredBy:     &triggeredBy,
		State:           verifydomain.StateQueued,
	}
	if err := d.UoW.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		return d.Runs.Create(ctx, tx, run)
	}); err != nil {
		return nil, err
	}

	payload, _ := json.Marshal(TaskPayload{CheckRunID: run.ID.String()})
	if d.Queue != nil {
		if _, err := d.Queue.Enqueue(ctx, ports.Task{
			Type:           ports.TaskRunCheck,
			Payload:        payload,
			IdempotencyKey: "check:" + run.ID.String() + ":1",
			MaxRetries:     3,
		}); err != nil {
			d.log().Error("check task enqueue failed",
				zap.String("check_run_id", run.ID.String()),
				zap.Error(err),
			)
		}
	}
	return run, nil
}

func (d Deps) GetCheckRun(ctx context.Context, actor Actor, id shared.CheckRunID) (*verifydomain.CheckRun, error) {
	run, err := d.Runs.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	lab, err := d.Labs.GetByID(ctx, run.LabInstanceID)
	if err != nil {
		return nil, err
	}
	if err := canLab(ctx, actor, identity.ActionLabRead, lab); err != nil {
		return nil, err
	}
	return run, nil
}

func (d Deps) ListChecksByLab(ctx context.Context, actor Actor, labID shared.LabInstanceID) ([]verifydomain.CheckRun, error) {
	lab, err := d.Labs.GetByID(ctx, labID)
	if err != nil {
		return nil, err
	}
	if err := canLab(ctx, actor, identity.ActionLabRead, lab); err != nil {
		return nil, err
	}
	return d.Runs.ListByLab(ctx, labID, 50)
}

func (d Deps) HandleTask(ctx context.Context, t ports.Task) error {
	var payload TaskPayload
	if err := json.Unmarshal(t.Payload, &payload); err != nil {
		return fmt.Errorf("verify: bad task payload: %w", err)
	}
	id, err := uuid.Parse(payload.CheckRunID)
	if err != nil {
		return fmt.Errorf("verify: bad check run id %q: %w", payload.CheckRunID, err)
	}
	return d.ExecuteCheck(ctx, shared.CheckRunID(id))
}

func (d Deps) ExecuteCheck(ctx context.Context, id shared.CheckRunID) error {
	run, err := d.Runs.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if run.State.IsTerminal() {
		return nil
	}
	lab, err := d.Labs.GetByID(ctx, run.LabInstanceID)
	if err != nil {
		return err
	}
	if len(lab.KIResources.FloatingIPs) == 0 || lab.CheckerSSHKeySecretID == nil {
		return d.finishErrored(ctx, run, "lab is missing target host or checker key", nil, "")
	}
	template, err := d.Templates.GetByID(ctx, run.CheckTemplateID)
	if err != nil {
		return err
	}
	key, err := d.Secrets.Get(ctx, *lab.CheckerSSHKeySecretID, "ssh_private_key", lab.ID.String())
	if err != nil {
		return err
	}
	defer zeroize(key)

	if run.State == verifydomain.StateQueued {
		if err := run.Start(d.now()); err != nil {
			return err
		}
		if err := d.save(ctx, run); err != nil {
			return err
		}
	}

	result, runErr := d.Runner.Run(ctx, ports.CheckRequest{
		PlaybookPath:   template.PlaybookPath,
		TargetHost:     lab.KIResources.FloatingIPs[0],
		SSHUser:        defaultSSHUser,
		SSHPrivateKey:  key,
		ExtraVars:      map[string]string{"lab_id": lab.ID.String(), "check_template": template.Slug},
		TimeoutSeconds: template.TimeoutSeconds,
	})
	final := result.State
	if final == "" || !final.IsTerminal() {
		final = verifydomain.StateErrored
	}
	summary := string(final)
	if runErr != nil {
		summary = runErr.Error()
	}
	if err := run.Finish(final, summary, result.Steps, result.Stdout, d.now()); err != nil {
		return err
	}
	if err := d.save(ctx, run); err != nil {
		return err
	}
	return nil
}

func (d Deps) finishErrored(ctx context.Context, run *verifydomain.CheckRun, summary string, steps []verifydomain.StepResult, stdout string) error {
	if run.State == verifydomain.StateQueued {
		if err := run.Start(d.now()); err != nil {
			return err
		}
	}
	if err := run.Finish(verifydomain.StateErrored, summary, steps, stdout, d.now()); err != nil {
		return err
	}
	return d.save(ctx, run)
}

func (d Deps) save(ctx context.Context, run *verifydomain.CheckRun) error {
	return d.UoW.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		return d.Runs.Save(ctx, tx, run)
	})
}

func (d Deps) now() time.Time {
	if d.Clock == nil {
		return time.Now().UTC()
	}
	return d.Clock.Now()
}

func (d Deps) log() *zap.Logger {
	if d.Logger == nil {
		return zap.NewNop()
	}
	return d.Logger
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

func zeroize(buf []byte) {
	for i := range buf {
		buf[i] = 0
	}
}
