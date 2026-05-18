package pool

import (
	"errors"
	"testing"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
)

func t0() time.Time { return time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC) }

func newFreeProject() *Project {
	return &Project{
		ID:                shared.NewProjectID(),
		KIProjectID:       "ki-proj-001",
		KIDomainID:        "domain-a",
		Name:              "test",
		State:             StateFree,
		LastStateChangeAt: t0(),
		CreatedAt:         t0(),
	}
}

func TestAllocateTo_FromFree(t *testing.T) {
	t.Parallel()
	p := newFreeProject()
	lab := shared.NewLabInstanceID()

	if err := p.AllocateTo(lab, t0()); err != nil {
		t.Fatalf("AllocateTo: %v", err)
	}
	if p.State != StateAllocated {
		t.Errorf("state %s, want allocated", p.State)
	}
	if p.AllocatedToLabID == nil || *p.AllocatedToLabID != lab {
		t.Errorf("allocated_to not set")
	}

	// Double-allocate should fail.
	err := p.AllocateTo(shared.NewLabInstanceID(), t0())
	var it shared.ErrInvalidTransition
	if !errors.As(err, &it) {
		t.Errorf("expected ErrInvalidTransition on double-allocate, got %v", err)
	}
}

func TestReleaseAndMarkClean(t *testing.T) {
	t.Parallel()
	p := newFreeProject()
	_ = p.AllocateTo(shared.NewLabInstanceID(), t0())

	if err := p.Release(t0()); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if p.State != StateCleaning {
		t.Errorf("state %s, want cleaning", p.State)
	}

	if err := p.MarkClean(t0()); err != nil {
		t.Fatalf("MarkClean: %v", err)
	}
	if p.State != StateFree {
		t.Errorf("state %s, want free", p.State)
	}
	if p.AllocatedToLabID != nil {
		t.Errorf("allocated_to should be cleared, got %v", p.AllocatedToLabID)
	}
	if p.CleanupFailures != 0 {
		t.Errorf("cleanup_failures should reset, got %d", p.CleanupFailures)
	}
}

func TestRecordCleanupFailure_QuarantinesAfterThreshold(t *testing.T) {
	t.Parallel()
	p := newFreeProject()
	_ = p.AllocateTo(shared.NewLabInstanceID(), t0())
	_ = p.Release(t0())

	for i := 1; i < MaxCleanupFailures; i++ {
		if err := p.RecordCleanupFailure("network error", t0()); err != nil {
			t.Fatalf("failure %d: %v", i, err)
		}
		if p.State != StateCleaning {
			t.Errorf("after failure %d state is %s, want cleaning", i, p.State)
		}
	}

	if err := p.RecordCleanupFailure("final straw", t0()); err != nil {
		t.Fatalf("final failure: %v", err)
	}
	if p.State != StateQuarantine {
		t.Errorf("expected quarantine after %d failures, got %s", MaxCleanupFailures, p.State)
	}
	if p.CleanupFailures != MaxCleanupFailures {
		t.Errorf("cleanup_failures = %d, want %d", p.CleanupFailures, MaxCleanupFailures)
	}
}

func TestQuarantineNow(t *testing.T) {
	t.Parallel()
	p := newFreeProject()
	if err := p.QuarantineNow("compromised", t0()); err != nil {
		t.Fatalf("QuarantineNow from free: %v", err)
	}
	if p.State != StateQuarantine {
		t.Errorf("state %s, want quarantine", p.State)
	}

	// Re-quarantine should be a no-op error.
	if err := p.QuarantineNow("again", t0()); err == nil {
		t.Errorf("expected error when re-quarantining")
	}
}

func TestManualResolve(t *testing.T) {
	t.Parallel()
	p := newFreeProject()
	_ = p.QuarantineNow("test", t0())

	if err := p.ManualResolve(t0()); err != nil {
		t.Fatalf("ManualResolve: %v", err)
	}
	if p.State != StateCleaning {
		t.Errorf("state %s, want cleaning", p.State)
	}
	if p.CleanupFailures != 0 {
		t.Errorf("cleanup_failures should reset, got %d", p.CleanupFailures)
	}
}

func TestPullEvents(t *testing.T) {
	t.Parallel()
	p := newFreeProject()
	_ = p.AllocateTo(shared.NewLabInstanceID(), t0())
	_ = p.Release(t0())
	_ = p.MarkClean(t0())

	events := p.PullEvents()
	if len(events) != 3 {
		t.Errorf("expected 3 events (allocated/releasing/freed), got %d", len(events))
	}
	if events[0].Kind() != "project.allocated" {
		t.Errorf("event[0].Kind = %s, want project.allocated", events[0].Kind())
	}

	if remaining := p.PullEvents(); len(remaining) != 0 {
		t.Errorf("expected 0 events after pull, got %d", len(remaining))
	}
}
