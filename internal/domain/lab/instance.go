package lab

import (
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

// LabInstance is the aggregate root of the Lab Lifecycle context.
// All state changes go through Transition; direct mutation of State is a bug.
type LabInstance struct {
	ID               shared.LabInstanceID
	StudentUserID    shared.UserID
	CourseID         shared.CourseID
	LabTemplateID    shared.LabTemplateID
	ProjectID        *shared.ProjectID

	State       State
	StateReason string

	// KI resources, populated as deploy saga progresses.
	KIResources KIResources

	// Lifecycle timers. Resolved against domain clock, not wall clock.
	CleanupAt  *time.Time
	UnfreezeAt *time.Time

	FrozenByUserID *shared.UserID
	FrozenReason   string

	StudentSSHKeySecretID *shared.SecretID
	CheckerSSHKeySecretID *shared.SecretID

	CreatedAt time.Time
	UpdatedAt time.Time

	// pendingEvents is the outbox: events appended during a transaction and
	// flushed by the repository when the aggregate is saved.
	pendingEvents []DomainEvent
}

// KIResources captures provider-side resource handles owned by this lab.
type KIResources struct {
	ServerIDs    []string
	NetworkID    string
	FloatingIPs  []string
	KeypairName  string
}

// New creates a new LabInstance in PendingQuota state.
func New(id shared.LabInstanceID, studentID shared.UserID, courseID shared.CourseID, templateID shared.LabTemplateID, now time.Time) *LabInstance {
	l := &LabInstance{
		ID:            id,
		StudentUserID: studentID,
		CourseID:      courseID,
		LabTemplateID: templateID,
		State:         StatePendingQuota,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	l.recordEvent(EventCreated{LabID: id, StudentID: studentID, CourseID: courseID, TemplateID: templateID})
	return l
}

// Transition moves the lab to a new state, validating against allowed
// transitions. Returns ErrInvalidTransition on illegal moves.
func (l *LabInstance) Transition(to State, reason string, now time.Time) error {
	if !CanTransition(l.State, to) {
		return shared.ErrInvalidTransition{Entity: "lab", From: string(l.State), To: string(to)}
	}
	from := l.State
	l.State = to
	l.StateReason = reason
	l.UpdatedAt = now
	l.recordEvent(EventStateChanged{LabID: l.ID, From: from, To: to, Reason: reason})
	return nil
}

// Freeze moves Ready → Frozen and sets unfreeze_at. Validates state.
func (l *LabInstance) Freeze(by shared.UserID, reason string, until time.Time, now time.Time) error {
	if err := l.Transition(StateFrozen, "freeze: "+reason, now); err != nil {
		return err
	}
	l.FrozenByUserID = &by
	l.FrozenReason = reason
	l.UnfreezeAt = &until
	l.CleanupAt = nil
	return nil
}

// Unfreeze moves Frozen → Ready and re-arms cleanup timer.
func (l *LabInstance) Unfreeze(cleanupAt time.Time, now time.Time) error {
	if err := l.Transition(StateReady, "unfreeze", now); err != nil {
		return err
	}
	l.FrozenByUserID = nil
	l.FrozenReason = ""
	l.UnfreezeAt = nil
	l.CleanupAt = &cleanupAt
	return nil
}

// ExtendCleanup pushes cleanup_at into the future.
func (l *LabInstance) ExtendCleanup(newCleanupAt time.Time, now time.Time) error {
	if l.State != StateReady {
		return shared.ErrInvalidTransition{Entity: "lab", From: string(l.State), To: "extend_cleanup"}
	}
	if l.CleanupAt != nil && newCleanupAt.Before(*l.CleanupAt) {
		return shared.ErrInvalidInput
	}
	l.CleanupAt = &newCleanupAt
	l.UpdatedAt = now
	l.recordEvent(EventCleanupExtended{LabID: l.ID, NewCleanupAt: newCleanupAt})
	return nil
}

// MarkReady is a convenience for the deploy saga: Deploying → Ready, sets cleanup_at.
func (l *LabInstance) MarkReady(cleanupAt time.Time, now time.Time) error {
	if err := l.Transition(StateReady, "deploy_succeeded", now); err != nil {
		return err
	}
	l.CleanupAt = &cleanupAt
	return nil
}

// AssignProject is called when PoolAllocator atomically reserves a project.
func (l *LabInstance) AssignProject(projectID shared.ProjectID, now time.Time) error {
	if l.State != StatePendingProject {
		return shared.ErrInvalidTransition{Entity: "lab", From: string(l.State), To: "assign_project"}
	}
	l.ProjectID = &projectID
	l.UpdatedAt = now
	l.recordEvent(EventProjectAssigned{LabID: l.ID, ProjectID: projectID})
	return l.Transition(StateDeploying, "project_assigned", now)
}

func (l *LabInstance) recordEvent(ev DomainEvent) {
	l.pendingEvents = append(l.pendingEvents, ev)
}

// PullEvents returns and clears the pending event buffer. Called by the
// repository inside the same transaction as the persistence write.
func (l *LabInstance) PullEvents() []DomainEvent {
	ev := l.pendingEvents
	l.pendingEvents = nil
	return ev
}
