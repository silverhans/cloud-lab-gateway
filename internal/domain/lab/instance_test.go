package lab

import (
	"errors"
	"testing"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

func t0() time.Time { return time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC) }

func newTestLab() *LabInstance {
	return New(shared.NewLabInstanceID(), shared.NewUserID(),
		shared.CourseID(shared.NewUserID()), // reuse uuid as CourseID for tests
		shared.LabTemplateID(shared.NewUserID()), t0())
}

func TestNew_StartsInPendingQuota(t *testing.T) {
	t.Parallel()
	l := newTestLab()
	if l.State != StatePendingQuota {
		t.Fatalf("got state %s, want pending_quota", l.State)
	}
	if got := l.PullEvents(); len(got) != 1 {
		t.Errorf("expected 1 EventCreated, got %d", len(got))
	}
}

func TestTransition_HappyPath(t *testing.T) {
	t.Parallel()
	l := newTestLab()
	_ = l.PullEvents() // discard create event

	want := []State{
		StatePendingProject, StateDeploying, StateReady,
		StateChecking, StateReady, StateCleaning, StateDone,
	}
	for _, s := range want {
		if err := l.Transition(s, "test", t0()); err != nil {
			t.Fatalf("transition %s → %s: %v", l.State, s, err)
		}
	}
}

func TestTransition_RejectsIllegal(t *testing.T) {
	t.Parallel()
	l := newTestLab()
	_ = l.PullEvents()

	// PendingQuota → Done is illegal
	err := l.Transition(StateDone, "skip", t0())
	var it shared.ErrInvalidTransition
	if !errors.As(err, &it) {
		t.Fatalf("want ErrInvalidTransition, got %v", err)
	}
	if it.From != "pending_quota" || it.To != "done" {
		t.Errorf("unexpected from/to: %+v", it)
	}
}

func TestFreezeUnfreeze(t *testing.T) {
	t.Parallel()
	l := newTestLab()
	mustTransition(t, l, StatePendingProject)
	mustTransition(t, l, StateDeploying)
	if err := l.MarkReady(t0().Add(2*time.Hour), t0()); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	_ = l.PullEvents()

	by := shared.NewUserID()
	until := t0().Add(24 * time.Hour)
	if err := l.Freeze(by, "broken vm", until, t0()); err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	if l.State != StateFrozen || l.UnfreezeAt == nil || l.FrozenByUserID == nil {
		t.Fatalf("freeze didn't set state/timers: %+v", l)
	}
	if l.CleanupAt != nil {
		t.Errorf("freeze should clear cleanup_at, got %v", l.CleanupAt)
	}

	newCleanup := t0().Add(3 * time.Hour)
	if err := l.Unfreeze(newCleanup, t0()); err != nil {
		t.Fatalf("Unfreeze: %v", err)
	}
	if l.State != StateReady {
		t.Errorf("expected Ready after unfreeze, got %s", l.State)
	}
	if l.UnfreezeAt != nil || l.FrozenByUserID != nil {
		t.Errorf("unfreeze should clear freeze fields")
	}
	if l.CleanupAt == nil || !l.CleanupAt.Equal(newCleanup) {
		t.Errorf("cleanup_at not re-armed: %v", l.CleanupAt)
	}
}

func TestExtendCleanup_OnlyInReady(t *testing.T) {
	t.Parallel()
	l := newTestLab()
	// Try in pending_quota — should fail
	err := l.ExtendCleanup(t0().Add(1*time.Hour), t0())
	if err == nil {
		t.Fatalf("expected error when extending non-ready lab")
	}

	mustTransition(t, l, StatePendingProject)
	mustTransition(t, l, StateDeploying)
	_ = l.MarkReady(t0().Add(1*time.Hour), t0())

	// Backwards extension rejected
	if err := l.ExtendCleanup(t0().Add(30*time.Minute), t0()); !errors.Is(err, shared.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for backwards extension, got %v", err)
	}

	// Forward extension OK
	newAt := t0().Add(3 * time.Hour)
	if err := l.ExtendCleanup(newAt, t0()); err != nil {
		t.Fatalf("ExtendCleanup: %v", err)
	}
	if !l.CleanupAt.Equal(newAt) {
		t.Errorf("cleanup_at not updated")
	}
}

func TestAssignProject(t *testing.T) {
	t.Parallel()
	l := newTestLab()
	mustTransition(t, l, StatePendingProject)
	_ = l.PullEvents()

	pid := shared.NewProjectID()
	if err := l.AssignProject(pid, t0()); err != nil {
		t.Fatalf("AssignProject: %v", err)
	}
	if l.State != StateDeploying {
		t.Errorf("expected Deploying, got %s", l.State)
	}
	if l.ProjectID == nil || *l.ProjectID != pid {
		t.Errorf("project id not set: %v", l.ProjectID)
	}

	events := l.PullEvents()
	if len(events) != 2 { // ProjectAssigned + StateChanged
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestAssignProject_WrongState(t *testing.T) {
	t.Parallel()
	l := newTestLab() // PendingQuota, not PendingProject
	err := l.AssignProject(shared.NewProjectID(), t0())
	if err == nil {
		t.Fatalf("expected error from wrong state")
	}
}

func TestPullEvents_ReturnsAndClears(t *testing.T) {
	t.Parallel()
	l := newTestLab()
	first := l.PullEvents()
	if len(first) != 1 {
		t.Errorf("expected 1 event on create, got %d", len(first))
	}
	second := l.PullEvents()
	if len(second) != 0 {
		t.Errorf("expected 0 events after pull, got %d", len(second))
	}
}

// mustTransition fails the test on illegal transition.
func mustTransition(t *testing.T, l *LabInstance, to State) {
	t.Helper()
	if err := l.Transition(to, "test", t0()); err != nil {
		t.Fatalf("transition %s → %s: %v", l.State, to, err)
	}
}
