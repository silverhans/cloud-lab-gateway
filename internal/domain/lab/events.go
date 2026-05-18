package lab

import "github.com/cloud-lab-gateway/gateway/internal/domain/shared"

// DomainEvent is the interface implemented by all events emitted by Lab aggregates.
// Events are flushed to the audit log + event bus in the same transaction as the
// state change (transactional outbox pattern).
type DomainEvent interface {
	Kind() string
}

type EventCreated struct {
	LabID      shared.LabInstanceID
	StudentID  shared.UserID
	CourseID   shared.CourseID
	TemplateID shared.LabTemplateID
}

func (EventCreated) Kind() string { return "lab.created" }

type EventStateChanged struct {
	LabID  shared.LabInstanceID
	From   State
	To     State
	Reason string
}

func (EventStateChanged) Kind() string { return "lab.state_changed" }

type EventProjectAssigned struct {
	LabID     shared.LabInstanceID
	ProjectID shared.ProjectID
}

func (EventProjectAssigned) Kind() string { return "lab.project_assigned" }

type EventCleanupExtended struct {
	LabID        shared.LabInstanceID
	NewCleanupAt interface{} // time.Time, kept as interface to avoid import cycle in audit
}

func (EventCleanupExtended) Kind() string { return "lab.cleanup_extended" }
