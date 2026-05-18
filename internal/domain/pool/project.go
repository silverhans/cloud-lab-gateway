// Package pool is the Pool & Capacity bounded context. It owns the Project
// aggregate (each Project is a pre-created КИ tenant available for rent).
package pool

import (
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

type State string

const (
	StateFree           State = "free"
	StateAllocated      State = "allocated"
	StateCleaning       State = "cleaning"
	StateQuarantine     State = "quarantine"
	StateDecommissioned State = "decommissioned"
)

// MaxCleanupFailures defines the threshold for moving a project to Quarantine.
const MaxCleanupFailures = 3

// Project is the aggregate root of the Pool context.
type Project struct {
	ID                shared.ProjectID
	KIProjectID       string
	KIDomainID        string
	Name              string
	State             State
	AllocatedToLabID  *shared.LabInstanceID
	CleanupFailures   int
	LastStateChangeAt time.Time
	CreatedAt         time.Time

	pendingEvents []DomainEvent
}

// AllocateTo moves Free → Allocated, binding this project to a lab instance.
// Caller is responsible for SELECT ... FOR UPDATE SKIP LOCKED outside this call.
func (p *Project) AllocateTo(labID shared.LabInstanceID, now time.Time) error {
	if p.State != StateFree {
		return shared.ErrInvalidTransition{Entity: "project", From: string(p.State), To: string(StateAllocated)}
	}
	p.State = StateAllocated
	p.AllocatedToLabID = &labID
	p.LastStateChangeAt = now
	p.recordEvent(EventAllocated{ProjectID: p.ID, LabID: labID})
	return nil
}

// Release moves Allocated → Cleaning. The actual resource cleanup happens
// asynchronously in the cleanup saga.
func (p *Project) Release(now time.Time) error {
	if p.State != StateAllocated {
		return shared.ErrInvalidTransition{Entity: "project", From: string(p.State), To: string(StateCleaning)}
	}
	p.State = StateCleaning
	p.LastStateChangeAt = now
	p.recordEvent(EventReleasing{ProjectID: p.ID, LabID: *p.AllocatedToLabID})
	return nil
}

// MarkClean is called by the cleanup saga upon success: Cleaning → Free.
func (p *Project) MarkClean(now time.Time) error {
	if p.State != StateCleaning {
		return shared.ErrInvalidTransition{Entity: "project", From: string(p.State), To: string(StateFree)}
	}
	p.State = StateFree
	p.AllocatedToLabID = nil
	p.CleanupFailures = 0
	p.LastStateChangeAt = now
	p.recordEvent(EventFreed{ProjectID: p.ID})
	return nil
}

// RecordCleanupFailure increments the failure counter. If it crosses
// MaxCleanupFailures, transitions to Quarantine.
func (p *Project) RecordCleanupFailure(reason string, now time.Time) error {
	if p.State != StateCleaning {
		return shared.ErrInvalidTransition{Entity: "project", From: string(p.State), To: "cleanup_failure"}
	}
	p.CleanupFailures++
	p.LastStateChangeAt = now
	if p.CleanupFailures >= MaxCleanupFailures {
		p.State = StateQuarantine
		p.recordEvent(EventQuarantined{ProjectID: p.ID, Reason: reason, Failures: p.CleanupFailures})
	}
	return nil
}

// QuarantineNow forcibly moves to Quarantine (admin action or critical incident).
func (p *Project) QuarantineNow(reason string, now time.Time) error {
	if p.State == StateDecommissioned || p.State == StateQuarantine {
		return shared.ErrInvalidTransition{Entity: "project", From: string(p.State), To: string(StateQuarantine)}
	}
	p.State = StateQuarantine
	p.LastStateChangeAt = now
	p.recordEvent(EventQuarantined{ProjectID: p.ID, Reason: reason, Failures: p.CleanupFailures})
	return nil
}

// ManualResolve moves Quarantine → Cleaning so the cleanup saga can re-run.
func (p *Project) ManualResolve(now time.Time) error {
	if p.State != StateQuarantine {
		return shared.ErrInvalidTransition{Entity: "project", From: string(p.State), To: string(StateCleaning)}
	}
	p.State = StateCleaning
	p.CleanupFailures = 0
	p.LastStateChangeAt = now
	return nil
}

func (p *Project) recordEvent(ev DomainEvent) {
	p.pendingEvents = append(p.pendingEvents, ev)
}

func (p *Project) PullEvents() []DomainEvent {
	ev := p.pendingEvents
	p.pendingEvents = nil
	return ev
}
