package deploy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cloud-lab-gateway/gateway/internal/adapters/cloud/inmem"
	labdomain "github.com/cloud-lab-gateway/gateway/internal/domain/lab"
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

type fakeSecretStore struct{ putCount int }

func (s *fakeSecretStore) Put(context.Context, string, string, []byte) (shared.SecretID, error) {
	s.putCount++
	return shared.NewSecretID(), nil
}
func (s *fakeSecretStore) Get(context.Context, shared.SecretID, string, string) ([]byte, error) {
	return nil, nil
}
func (s *fakeSecretStore) Delete(context.Context, shared.SecretID) error { return nil }

type fakeLabRepo struct {
	lab   *labdomain.LabInstance
	saved int
}

func (r *fakeLabRepo) GetByID(context.Context, shared.LabInstanceID) (*labdomain.LabInstance, error) {
	if r.lab == nil {
		return nil, shared.ErrNotFound
	}
	return r.lab, nil
}
func (r *fakeLabRepo) Create(context.Context, ports.Tx, *labdomain.LabInstance) error { return nil }
func (r *fakeLabRepo) Save(_ context.Context, _ ports.Tx, l *labdomain.LabInstance) error {
	r.lab = l
	r.saved++
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

type fakeStepRepo struct {
	steps map[string]ports.DeployStep
}

func newFakeStepRepo() *fakeStepRepo {
	return &fakeStepRepo{steps: map[string]ports.DeployStep{}}
}
func stepKey(labID shared.LabInstanceID, step string) string {
	return labID.String() + ":" + step
}
func (r *fakeStepRepo) GetOrInit(_ context.Context, labID shared.LabInstanceID, step string) (ports.DeployStep, error) {
	if s, ok := r.steps[stepKey(labID, step)]; ok {
		return s, nil
	}
	return ports.DeployStep{LabID: labID, StepName: step, Status: "pending"}, nil
}
func (r *fakeStepRepo) Save(_ context.Context, s ports.DeployStep) error {
	r.steps[stepKey(s.LabID, s.StepName)] = s
	return nil
}
func (r *fakeStepRepo) ListByLab(_ context.Context, labID shared.LabInstanceID) ([]ports.DeployStep, error) {
	out := make([]ports.DeployStep, 0)
	for _, s := range r.steps {
		if s.LabID == labID {
			out = append(out, s)
		}
	}
	return out, nil
}

type fakeQueue struct{ enqueued []ports.Task }

func (q *fakeQueue) Enqueue(_ context.Context, t ports.Task) (string, error) {
	q.enqueued = append(q.enqueued, t)
	return t.IdempotencyKey, nil
}
func (q *fakeQueue) EnqueueAt(_ context.Context, t ports.Task, _ time.Time) (string, error) {
	q.enqueued = append(q.enqueued, t)
	return t.IdempotencyKey, nil
}
func (q *fakeQueue) Cancel(context.Context, string) error { return nil }

// --- harness ----------------------------------------------------------------

type harness struct {
	cloud    ports.CloudProvider
	labRepo  *fakeLabRepo
	stepRepo *fakeStepRepo
	secrets  *fakeSecretStore
	queue    *fakeQueue
	clock    fakeClock
	lab      *labdomain.LabInstance
}

func deployingLab(now time.Time) *labdomain.LabInstance {
	l := labdomain.New(shared.NewLabInstanceID(), shared.NewUserID(),
		shared.CourseID(uuid.New()), shared.LabTemplateID(uuid.New()), now)
	_ = l.Transition(labdomain.StatePendingProject, "quota_ok", now)
	_ = l.AssignProject(shared.NewProjectID(), now) // → deploying
	l.PullEvents()
	return l
}

func newHarness() *harness {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	lab := deployingLab(now)
	return &harness{
		cloud:    inmem.New(inmem.DefaultCapacity(), inmem.Faults{}),
		labRepo:  &fakeLabRepo{lab: lab},
		stepRepo: newFakeStepRepo(),
		secrets:  &fakeSecretStore{},
		queue:    &fakeQueue{},
		clock:    fakeClock{t: now},
		lab:      lab,
	}
}

func (h *harness) deps() Deps {
	return Deps{
		Cloud:        h.cloud,
		Lab:          h.labRepo,
		Steps:        h.stepRepo,
		Secrets:      h.secrets,
		Queue:        h.queue,
		UoW:          fakeUoW{},
		Clock:        h.clock,
		CleanupAfter: 2 * time.Hour,
	}
}

func (h *harness) hasTask(typ ports.TaskType, idemSuffix string) bool {
	for _, t := range h.queue.enqueued {
		if t.Type == typ && (idemSuffix == "" || endsWith(t.IdempotencyKey, idemSuffix)) {
			return true
		}
	}
	return false
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// --- tests ------------------------------------------------------------------

func TestRun_HappyPath(t *testing.T) {
	t.Parallel()
	h := newHarness()

	if err := h.deps().Run(context.Background(), h.lab.ID); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got := h.labRepo.lab
	if got.State != labdomain.StateReady {
		t.Fatalf("state = %s, want ready", got.State)
	}
	if got.CleanupAt == nil {
		t.Error("cleanup_at not armed")
	}
	if got.KIResources.NetworkID == "" || len(got.KIResources.ServerIDs) == 0 ||
		len(got.KIResources.FloatingIPs) == 0 || got.KIResources.KeypairName == "" {
		t.Errorf("KIResources not fully populated: %+v", got.KIResources)
	}
	// Lab stand #3 is a five-machine topology — the saga must boot every VM
	// and attach a floating IP to each so the Ansible checker can reach it.
	if n := len(got.KIResources.ServerIDs); n != 5 {
		t.Errorf("ServerIDs count = %d, want 5 (lab stand #3 topology)", n)
	}
	if n := len(got.KIResources.FloatingIPs); n != 5 {
		t.Errorf("FloatingIPs count = %d, want 5", n)
	}
	if got.CheckerSSHKeySecretID == nil {
		t.Error("checker ssh key secret not set")
	}
	if h.secrets.putCount != 1 {
		t.Errorf("SecretStore.Put called %d times, want 1", h.secrets.putCount)
	}
	if !h.hasTask(ports.TaskCleanupLab, ":timer") {
		t.Error("auto-cleanup timer task not scheduled")
	}
	steps, _ := h.stepRepo.ListByLab(context.Background(), h.lab.ID)
	if len(steps) != 5 {
		t.Errorf("expected 5 step records, got %d", len(steps))
	}
	for _, s := range steps {
		if s.Status != stepSucceeded {
			t.Errorf("step %s status = %s, want succeeded", s.StepName, s.Status)
		}
	}
}

func TestRun_ResumesSkippingCompletedSteps(t *testing.T) {
	t.Parallel()
	h := newHarness()
	ks := shared.NewSecretID().String()

	// Pre-seed the first two steps as already succeeded, with Attempt=7 as a
	// marker — if the saga re-runs them the attempt counter would change.
	_ = h.stepRepo.Save(context.Background(), ports.DeployStep{
		LabID: h.lab.ID, StepName: string(stepCreateKeypair), Status: stepSucceeded, Attempt: 7,
		Result: []byte(`{"keypair_name":"preset-key","key_secret_id":"` + ks + `"}`),
	})
	_ = h.stepRepo.Save(context.Background(), ports.DeployStep{
		LabID: h.lab.ID, StepName: string(stepProvisionNetwork), Status: stepSucceeded, Attempt: 7,
		Result: []byte(`{"keypair_name":"preset-key","key_secret_id":"` + ks + `","network_id":"preset-net"}`),
	})

	if err := h.deps().Run(context.Background(), h.lab.ID); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if h.labRepo.lab.State != labdomain.StateReady {
		t.Fatalf("state = %s, want ready", h.labRepo.lab.State)
	}
	// The skipped steps must be untouched.
	for _, name := range []stepName{stepCreateKeypair, stepProvisionNetwork} {
		rec := h.stepRepo.steps[stepKey(h.lab.ID, string(name))]
		if rec.Attempt != 7 {
			t.Errorf("step %s was re-run (attempt %d), should have been skipped", name, rec.Attempt)
		}
	}
	// createKeypair skipped → SecretStore must not have been touched.
	if h.secrets.putCount != 0 {
		t.Errorf("SecretStore.Put called %d times, want 0 (step skipped)", h.secrets.putCount)
	}
	// The replayed state must carry through to the final resources.
	if h.labRepo.lab.KIResources.NetworkID != "preset-net" {
		t.Errorf("replayed network id lost: %q", h.labRepo.lab.KIResources.NetworkID)
	}
}

func TestRun_TransientFailureRetries(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.cloud = inmem.New(inmem.DefaultCapacity(), inmem.Faults{
		BootServerErr: errors.New("nova timeout"),
	})

	err := h.deps().Run(context.Background(), h.lab.ID)
	if err == nil {
		t.Fatal("expected an error so asynq retries the task")
	}
	// Lab stays in deploying — not failed yet, retries remain.
	if h.labRepo.lab != nil && h.labRepo.lab.State == labdomain.StateFailed {
		t.Error("lab should not be failed after a single transient failure")
	}
	boot := h.stepRepo.steps[stepKey(h.lab.ID, string(stepBootVM))]
	if boot.Status != stepFailed || boot.Attempt != 1 {
		t.Errorf("boot_vm record = %+v, want failed/attempt=1", boot)
	}
}

func TestRun_TerminalFailureCompensates(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.cloud = inmem.New(inmem.DefaultCapacity(), inmem.Faults{
		BootServerErr: errors.New("nova permanently down"),
	})
	d := h.deps()

	// Attempts 1 and 2 return an error (asynq would retry).
	for i := 1; i <= 2; i++ {
		if err := d.Run(context.Background(), h.lab.ID); err == nil {
			t.Fatalf("attempt %d: expected retryable error", i)
		}
	}
	// Attempt 3 exhausts the budget → compensate, return nil.
	if err := d.Run(context.Background(), h.lab.ID); err != nil {
		t.Fatalf("attempt 3 should compensate and return nil, got %v", err)
	}
	if h.labRepo.lab.State != labdomain.StateFailed {
		t.Fatalf("state = %s, want failed", h.labRepo.lab.State)
	}
	if !h.hasTask(ports.TaskCleanupLab, ":compensate") {
		t.Error("compensation cleanup task not enqueued")
	}
	// createKeypair succeeded once and was never re-run across the retries.
	if h.secrets.putCount != 1 {
		t.Errorf("SecretStore.Put called %d times across retries, want 1", h.secrets.putCount)
	}
}

func TestRun_LabNotDeployingIsNoop(t *testing.T) {
	t.Parallel()
	h := newHarness()
	// Force the lab into a non-deploying state.
	_ = h.lab.Transition(labdomain.StateFailed, "external", h.clock.t)
	h.lab.PullEvents()

	if err := h.deps().Run(context.Background(), h.lab.ID); err != nil {
		t.Fatalf("Run on non-deploying lab should be a no-op, got %v", err)
	}
	steps, _ := h.stepRepo.ListByLab(context.Background(), h.lab.ID)
	if len(steps) != 0 {
		t.Errorf("no steps should run for a non-deploying lab, got %d", len(steps))
	}
}

func TestHandleTask_DecodesPayload(t *testing.T) {
	t.Parallel()
	h := newHarness()
	payload := []byte(`{"lab_id":"` + h.lab.ID.String() + `"}`)

	if err := h.deps().HandleTask(context.Background(), ports.Task{
		Type:    ports.TaskDeployLab,
		Payload: payload,
	}); err != nil {
		t.Fatalf("HandleTask: %v", err)
	}
	if h.labRepo.lab.State != labdomain.StateReady {
		t.Errorf("state = %s, want ready", h.labRepo.lab.State)
	}
}

func TestHandleTask_BadPayload(t *testing.T) {
	t.Parallel()
	h := newHarness()
	if err := h.deps().HandleTask(context.Background(), ports.Task{Payload: []byte(`not json`)}); err == nil {
		t.Error("expected error on malformed payload")
	}
}
