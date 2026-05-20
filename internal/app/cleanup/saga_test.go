package cleanup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cloud-lab-gateway/gateway/internal/adapters/cloud/inmem"
	labdomain "github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	pooldomain "github.com/cloud-lab-gateway/gateway/internal/domain/pool"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

// --- fakes ------------------------------------------------------------------

type fakeTx struct{}

func (fakeTx) Private() {}

type fakeUoW struct{}

func (fakeUoW) WithTx(ctx context.Context, fn func(context.Context, ports.Tx) error) error {
	return fn(ctx, fakeTx{})
}

type fakeClock struct{ t time.Time }

func (c fakeClock) Now() time.Time { return c.t }

type fakeSecretStore struct{ deleted int }

func (s *fakeSecretStore) Put(context.Context, string, string, []byte) (shared.SecretID, error) {
	return shared.NewSecretID(), nil
}
func (s *fakeSecretStore) Get(context.Context, shared.SecretID, string, string) ([]byte, error) {
	return nil, nil
}
func (s *fakeSecretStore) Delete(context.Context, shared.SecretID) error {
	s.deleted++
	return nil
}

type fakeLabRepo struct{ lab *labdomain.LabInstance }

func (r *fakeLabRepo) GetByID(context.Context, shared.LabInstanceID) (*labdomain.LabInstance, error) {
	if r.lab == nil {
		return nil, shared.ErrNotFound
	}
	return r.lab, nil
}
func (r *fakeLabRepo) Create(context.Context, ports.Tx, *labdomain.LabInstance) error { return nil }
func (r *fakeLabRepo) Save(_ context.Context, _ ports.Tx, l *labdomain.LabInstance) error {
	r.lab = l
	return nil
}
func (r *fakeLabRepo) FindActiveByStudentAndCourse(context.Context, shared.UserID, shared.CourseID) (*labdomain.LabInstance, error) {
	return nil, nil
}
func (r *fakeLabRepo) ListPendingCleanup(context.Context, time.Time, int) ([]labdomain.LabInstance, error) {
	return nil, nil
}
func (r *fakeLabRepo) ListPendingUnfreeze(context.Context, time.Time, int) ([]labdomain.LabInstance, error) {
	return nil, nil
}

type fakePoolRepo struct{ project *pooldomain.Project }

func (r *fakePoolRepo) AllocateOneFree(context.Context, ports.Tx, string, shared.LabInstanceID) (*pooldomain.Project, error) {
	return nil, shared.ErrPoolEmpty
}
func (r *fakePoolRepo) Save(_ context.Context, _ ports.Tx, p *pooldomain.Project) error {
	r.project = p
	return nil
}
func (r *fakePoolRepo) GetByID(context.Context, shared.ProjectID) (*pooldomain.Project, error) {
	if r.project == nil {
		return nil, shared.ErrNotFound
	}
	return r.project, nil
}
func (r *fakePoolRepo) ListByDomain(context.Context, string, *pooldomain.State) ([]pooldomain.Project, error) {
	return nil, nil
}
func (r *fakePoolRepo) SeedInsert(context.Context, []pooldomain.Project) error { return nil }

// --- harness ----------------------------------------------------------------

type harness struct {
	cloud   ports.CloudProvider
	labRepo *fakeLabRepo
	pool    *fakePoolRepo
	secrets *fakeSecretStore
	clock   fakeClock
	lab     *labdomain.LabInstance
}

// readyLab builds a lab in `ready` state with provisioned КИ resources and an
// allocated project behind it.
func newHarness(state labdomain.State) *harness {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	projectID := shared.NewProjectID()

	l := labdomain.New(shared.NewLabInstanceID(), shared.NewUserID(),
		shared.CourseID(uuid.New()), shared.LabTemplateID(uuid.New()), now)
	_ = l.Transition(labdomain.StatePendingProject, "", now)
	_ = l.AssignProject(projectID, now) // → deploying
	switch state {
	case labdomain.StateReady:
		_ = l.MarkReady(now.Add(2*time.Hour), now)
	case labdomain.StateFailed:
		_ = l.Transition(labdomain.StateFailed, "deploy failed", now)
	}
	secretID := shared.NewSecretID()
	l.KIResources = labdomain.KIResources{
		ServerIDs:   []string{"srv-1"},
		NetworkID:   "net-1",
		FloatingIPs: []string{"203.0.113.10"},
		KeypairName: "lab-key",
	}
	l.CheckerSSHKeySecretID = &secretID
	l.PullEvents()

	return &harness{
		cloud:   inmem.New(inmem.DefaultCapacity(), inmem.Faults{}),
		labRepo: &fakeLabRepo{lab: l},
		pool: &fakePoolRepo{project: &pooldomain.Project{
			ID:               projectID,
			KIDomainID:       "dom-1",
			State:            pooldomain.StateAllocated,
			AllocatedToLabID: idPtr(l.ID),
		}},
		secrets: &fakeSecretStore{},
		clock:   fakeClock{t: now},
		lab:     l,
	}
}

func idPtr(id shared.LabInstanceID) *shared.LabInstanceID { return &id }

func (h *harness) deps() Deps {
	return Deps{
		Cloud:   h.cloud,
		Lab:     h.labRepo,
		Pool:    h.pool,
		Secrets: h.secrets,
		UoW:     fakeUoW{},
		Clock:   h.clock,
	}
}

// --- tests ------------------------------------------------------------------

func TestRun_HappyPath_ReadyLab(t *testing.T) {
	t.Parallel()
	h := newHarness(labdomain.StateReady)

	if err := h.deps().Run(context.Background(), h.lab.ID); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if h.labRepo.lab.State != labdomain.StateDone {
		t.Errorf("lab state = %s, want done", h.labRepo.lab.State)
	}
	if h.pool.project.State != pooldomain.StateFree {
		t.Errorf("project state = %s, want free", h.pool.project.State)
	}
	if h.secrets.deleted != 1 {
		t.Errorf("ssh secret deleted %d times, want 1", h.secrets.deleted)
	}
}

func TestRun_CompensatesFailedLab(t *testing.T) {
	t.Parallel()
	h := newHarness(labdomain.StateFailed)

	if err := h.deps().Run(context.Background(), h.lab.ID); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if h.labRepo.lab.State != labdomain.StateDone {
		t.Errorf("failed lab not cleaned: state = %s", h.labRepo.lab.State)
	}
	if h.pool.project.State != pooldomain.StateFree {
		t.Errorf("project not returned to pool: state = %s", h.pool.project.State)
	}
}

func TestRun_TeardownFailureRetries(t *testing.T) {
	t.Parallel()
	h := newHarness(labdomain.StateReady)
	h.cloud = inmem.New(inmem.DefaultCapacity(), inmem.Faults{
		DeleteServerErr: errors.New("nova delete timeout"),
	})

	err := h.deps().Run(context.Background(), h.lab.ID)
	if err == nil {
		t.Fatal("expected a retryable error on teardown failure")
	}
	if h.pool.project.State != pooldomain.StateCleaning {
		t.Errorf("project state = %s, want cleaning (mid-retry)", h.pool.project.State)
	}
	if h.pool.project.CleanupFailures != 1 {
		t.Errorf("cleanup_failures = %d, want 1", h.pool.project.CleanupFailures)
	}
}

func TestRun_QuarantinesAfterRepeatedFailures(t *testing.T) {
	t.Parallel()
	h := newHarness(labdomain.StateReady)
	h.cloud = inmem.New(inmem.DefaultCapacity(), inmem.Faults{
		DeleteServerErr: errors.New("nova permanently down"),
	})
	d := h.deps()

	// Two retryable failures.
	for i := 1; i <= 2; i++ {
		if err := d.Run(context.Background(), h.lab.ID); err == nil {
			t.Fatalf("attempt %d: expected retryable error", i)
		}
	}
	// Third failure trips MaxCleanupFailures → quarantine, saga stops.
	if err := d.Run(context.Background(), h.lab.ID); err != nil {
		t.Fatalf("attempt 3 should quarantine and return nil, got %v", err)
	}
	if h.pool.project.State != pooldomain.StateQuarantine {
		t.Errorf("project state = %s, want quarantine", h.pool.project.State)
	}
	if h.labRepo.lab.State != labdomain.StateDone {
		t.Errorf("lab state = %s, want done", h.labRepo.lab.State)
	}
}

func TestRun_AlreadyDoneIsNoop(t *testing.T) {
	t.Parallel()
	h := newHarness(labdomain.StateReady)
	_ = h.lab.Transition(labdomain.StateCleaning, "", h.clock.t)
	_ = h.lab.Transition(labdomain.StateDone, "", h.clock.t)
	h.lab.PullEvents()

	if err := h.deps().Run(context.Background(), h.lab.ID); err != nil {
		t.Fatalf("Run on done lab should be a no-op, got %v", err)
	}
	if h.secrets.deleted != 0 {
		t.Errorf("done lab should not be touched, secret deleted %d times", h.secrets.deleted)
	}
}

func TestHandleTask_DecodesPayload(t *testing.T) {
	t.Parallel()
	h := newHarness(labdomain.StateReady)
	payload := []byte(`{"lab_id":"` + h.lab.ID.String() + `"}`)

	if err := h.deps().HandleTask(context.Background(), ports.Task{
		Type:    ports.TaskCleanupLab,
		Payload: payload,
	}); err != nil {
		t.Fatalf("HandleTask: %v", err)
	}
	if h.labRepo.lab.State != labdomain.StateDone {
		t.Errorf("lab state = %s, want done", h.labRepo.lab.State)
	}
}
