// Package verify is the Verification bounded context. It owns CheckTemplate
// (description of an Ansible check) and CheckRun (a single execution).
package verify

import (
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

type RunState string

const (
	StateQueued  RunState = "queued"
	StateRunning RunState = "running"
	StatePassed  RunState = "passed"
	StateFailed  RunState = "failed"
	StateTimeout RunState = "timeout"
	StateErrored RunState = "errored"
)

func (s RunState) IsTerminal() bool {
	return s == StatePassed || s == StateFailed || s == StateTimeout || s == StateErrored
}

type CheckRun struct {
	ID              shared.CheckRunID
	LabInstanceID   shared.LabInstanceID
	CheckTemplateID shared.CheckTemplate
	TriggeredBy     *shared.UserID

	State      RunState
	StartedAt  *time.Time
	FinishedAt *time.Time
	Summary    string

	AnsibleStdout string
	Steps         []StepResult
}

type StepStatus string

const (
	StepOK          StepStatus = "ok"
	StepChanged     StepStatus = "changed"
	StepFailed      StepStatus = "failed"
	StepUnreachable StepStatus = "unreachable"
	StepSkipped     StepStatus = "skipped"
)

type StepResult struct {
	TaskName string
	Status   StepStatus
	Expected interface{}
	Actual   interface{}
	Message  string
}

// Start transitions Queued → Running.
func (r *CheckRun) Start(now time.Time) error {
	if r.State != StateQueued {
		return shared.ErrInvalidTransition{Entity: "check_run", From: string(r.State), To: string(StateRunning)}
	}
	r.State = StateRunning
	r.StartedAt = &now
	return nil
}

// Finish transitions Running → terminal state.
func (r *CheckRun) Finish(final RunState, summary string, steps []StepResult, stdout string, now time.Time) error {
	if r.State != StateRunning {
		return shared.ErrInvalidTransition{Entity: "check_run", From: string(r.State), To: string(final)}
	}
	if !final.IsTerminal() {
		return shared.ErrInvalidInput
	}
	r.State = final
	r.Summary = summary
	r.Steps = steps
	r.AnsibleStdout = stdout
	r.FinishedAt = &now
	return nil
}
