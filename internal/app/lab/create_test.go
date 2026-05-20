package lab

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cloud-lab-gateway/gateway/internal/domain/audit"
	"github.com/cloud-lab-gateway/gateway/internal/domain/identity"
	labdomain "github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	"github.com/cloud-lab-gateway/gateway/internal/domain/pool"
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

type fakeQuotaCache struct {
	snap shared.QuotaSnapshot
	age  time.Duration
	err  error
}

func (q fakeQuotaCache) Read(context.Context) (shared.QuotaSnapshot, time.Duration, error) {
	return q.snap, q.age, q.err
}
func (fakeQuotaCache) Write(context.Context, shared.QuotaSnapshot) error { return nil }

type fakeCourseRepo struct {
	course *identity.Course
	err    error
}

func (r fakeCourseRepo) GetByID(context.Context, shared.CourseID) (*identity.Course, error) {
	return r.course, r.err
}
func (fakeCourseRepo) GetByExternalID(context.Context, string) (*identity.Course, error) {
	return nil, shared.ErrNotFound
}
func (fakeCourseRepo) Upsert(context.Context, ports.Tx, *identity.Course) error    { return nil }
func (fakeCourseRepo) Enroll(context.Context, ports.Tx, identity.Enrollment) error { return nil }

type fakeLabRepo struct {
	active    *labdomain.LabInstance
	findErr   error
	createErr error
	created   []*labdomain.LabInstance
	saved     []*labdomain.LabInstance
}

func (r *fakeLabRepo) Create(_ context.Context, _ ports.Tx, l *labdomain.LabInstance) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.created = append(r.created, l)
	return nil
}
func (r *fakeLabRepo) Save(_ context.Context, _ ports.Tx, l *labdomain.LabInstance) error {
	r.saved = append(r.saved, l)
	return nil
}
func (r *fakeLabRepo) GetByID(context.Context, shared.LabInstanceID) (*labdomain.LabInstance, error) {
	return nil, shared.ErrNotFound
}
func (r *fakeLabRepo) FindActiveByStudentAndCourse(context.Context, shared.UserID, shared.CourseID) (*labdomain.LabInstance, error) {
	return r.active, r.findErr
}
func (r *fakeLabRepo) ListPendingCleanup(context.Context, time.Time, int) ([]labdomain.LabInstance, error) {
	return nil, nil
}
func (r *fakeLabRepo) ListPendingUnfreeze(context.Context, time.Time, int) ([]labdomain.LabInstance, error) {
	return nil, nil
}

type fakePoolRepo struct {
	project  *pool.Project
	allocErr error
	saved    []*pool.Project
}

func (r *fakePoolRepo) AllocateOneFree(context.Context, ports.Tx, string, shared.LabInstanceID) (*pool.Project, error) {
	if r.allocErr != nil {
		return nil, r.allocErr
	}
	return r.project, nil
}
func (r *fakePoolRepo) Save(_ context.Context, _ ports.Tx, p *pool.Project) error {
	r.saved = append(r.saved, p)
	return nil
}
func (r *fakePoolRepo) GetByID(context.Context, shared.ProjectID) (*pool.Project, error) {
	return nil, shared.ErrNotFound
}
func (r *fakePoolRepo) ListByDomain(context.Context, string, *pool.State) ([]pool.Project, error) {
	return nil, nil
}
func (r *fakePoolRepo) SeedInsert(context.Context, []pool.Project) error { return nil }

type fakeAudit struct {
	events []audit.AuditEvent
}

func (a *fakeAudit) Append(_ context.Context, ev audit.AuditEvent) error {
	a.events = append(a.events, ev)
	return nil
}
func (a *fakeAudit) AppendInTx(_ context.Context, _ ports.Tx, ev audit.AuditEvent) error {
	a.events = append(a.events, ev)
	return nil
}
func (a *fakeAudit) Query(context.Context, ports.AuditFilter) ([]audit.AuditEvent, error) {
	return a.events, nil
}

type fakeQueue struct {
	enqueued []ports.Task
}

func (q *fakeQueue) Enqueue(_ context.Context, t ports.Task) (string, error) {
	q.enqueued = append(q.enqueued, t)
	return "task-" + string(t.Type), nil
}
func (q *fakeQueue) EnqueueAt(_ context.Context, t ports.Task, _ time.Time) (string, error) {
	q.enqueued = append(q.enqueued, t)
	return "task-" + string(t.Type), nil
}
func (q *fakeQueue) Cancel(context.Context, string) error { return nil }

// --- harness ----------------------------------------------------------------

func lowUtilSnapshot() shared.QuotaSnapshot {
	return shared.QuotaSnapshot{
		VCPUs: shared.Capacity{Used: 10, Total: 100},
		RAM:   shared.Capacity{Used: 10000, Total: 100000},
		Disk:  shared.Capacity{Used: 100, Total: 1000},
	}
}

type harness struct {
	pool    *fakePoolRepo
	lab     *fakeLabRepo
	courses fakeCourseRepo
	audit   *fakeAudit
	quota   fakeQuotaCache
	queue   *fakeQueue
	clock   fakeClock
}

func newHarness() *harness {
	return &harness{
		pool:    &fakePoolRepo{project: &pool.Project{ID: shared.NewProjectID(), State: pool.StateAllocated}},
		lab:     &fakeLabRepo{},
		courses: fakeCourseRepo{course: &identity.Course{ID: shared.CourseID(uuid.New()), KIDomainID: "dom-1", Name: "Linux"}},
		audit:   &fakeAudit{},
		quota:   fakeQuotaCache{snap: lowUtilSnapshot(), age: 0},
		queue:   &fakeQueue{},
		clock:   fakeClock{t: time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)},
	}
}

func (h *harness) deps() Deps {
	return Deps{
		UoW: fakeUoW{}, Pool: h.pool, Lab: h.lab, Courses: h.courses,
		Audit: h.audit, QuotaCache: h.quota, Queue: h.queue, Clock: h.clock,
	}
}

func sampleInput() CreateInput {
	return CreateInput{
		StudentUserID: shared.NewUserID(),
		CourseID:      shared.CourseID(uuid.New()),
		LabTemplateID: shared.LabTemplateID(uuid.New()),
		RequestID:     "req-1",
	}
}

func countTasks(tasks []ports.Task, typ ports.TaskType) int {
	n := 0
	for _, t := range tasks {
		if t.Type == typ {
			n++
		}
	}
	return n
}

// --- tests ------------------------------------------------------------------

func TestCreateLab_HappyPath(t *testing.T) {
	t.Parallel()
	h := newHarness()

	inst, err := h.deps().CreateLab(context.Background(), sampleInput())
	if err != nil {
		t.Fatalf("CreateLab: %v", err)
	}
	if inst.State != labdomain.StateDeploying {
		t.Errorf("state = %s, want deploying", inst.State)
	}
	if len(h.lab.created) != 1 {
		t.Errorf("expected 1 lab created, got %d", len(h.lab.created))
	}
	if len(h.pool.saved) != 1 {
		t.Errorf("expected project saved once, got %d", len(h.pool.saved))
	}
	if n := countTasks(h.queue.enqueued, ports.TaskDeployLab); n != 1 {
		t.Errorf("expected 1 deploy task, got %d", n)
	}
}

func TestCreateLab_QuotaExceeded(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.quota.snap = shared.QuotaSnapshot{
		VCPUs: shared.Capacity{Used: 95, Total: 100},
		RAM:   shared.Capacity{Used: 100, Total: 100000},
		Disk:  shared.Capacity{Used: 100, Total: 1000},
	}

	_, err := h.deps().CreateLab(context.Background(), sampleInput())
	if !errors.Is(err, shared.ErrQuotaExceeded) {
		t.Fatalf("expected ErrQuotaExceeded, got %v", err)
	}
	if len(h.lab.created) != 0 {
		t.Errorf("no lab should be created on quota block, got %d", len(h.lab.created))
	}
	if countTasks(h.queue.enqueued, ports.TaskDeployLab) != 0 {
		t.Errorf("no deploy task should be enqueued on quota block")
	}
	if len(h.audit.events) != 1 || h.audit.events[0].Kind != audit.KindQuotaBlocked {
		t.Errorf("expected one quota.blocked audit event, got %+v", h.audit.events)
	}
}

func TestCreateLab_PoolEmpty(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.pool.allocErr = shared.ErrPoolEmpty

	_, err := h.deps().CreateLab(context.Background(), sampleInput())
	if !errors.Is(err, shared.ErrPoolEmpty) {
		t.Fatalf("expected ErrPoolEmpty, got %v", err)
	}
	// The lab is created then committed in `rejected` state for history.
	if len(h.lab.created) != 1 {
		t.Errorf("expected lab created, got %d", len(h.lab.created))
	}
	if len(h.lab.saved) != 1 || h.lab.saved[0].State != labdomain.StateRejected {
		t.Errorf("expected lab saved as rejected, got %+v", h.lab.saved)
	}
	if len(h.audit.events) != 1 || h.audit.events[0].Kind != audit.KindQuotaBlocked {
		t.Errorf("expected one audit event for pool-empty, got %+v", h.audit.events)
	}
	if countTasks(h.queue.enqueued, ports.TaskDeployLab) != 0 {
		t.Errorf("no deploy task should be enqueued when pool is empty")
	}
}

func TestCreateLab_LabAlreadyActive(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.lab.active = labdomain.New(shared.NewLabInstanceID(), shared.NewUserID(),
		shared.CourseID(uuid.New()), shared.LabTemplateID(uuid.New()), h.clock.t)

	_, err := h.deps().CreateLab(context.Background(), sampleInput())
	if !errors.Is(err, shared.ErrLabAlreadyActive) {
		t.Fatalf("expected ErrLabAlreadyActive, got %v", err)
	}
}

func TestCreateLab_QuotaCacheStale(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.quota.age = 5 * time.Minute // older than the 60s ceiling

	_, err := h.deps().CreateLab(context.Background(), sampleInput())
	if !errors.Is(err, shared.ErrCloudUnavailable) {
		t.Fatalf("expected ErrCloudUnavailable, got %v", err)
	}
	if n := countTasks(h.queue.enqueued, ports.TaskRefreshQuota); n != 1 {
		t.Errorf("expected a refresh-quota task to be enqueued, got %d", n)
	}
	if len(h.lab.created) != 0 {
		t.Errorf("no lab should be created on stale cache")
	}
}

func TestCreateLab_CourseNotFoundPropagates(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.courses.err = shared.ErrNotFound

	_, err := h.deps().CreateLab(context.Background(), sampleInput())
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("expected ErrNotFound to propagate, got %v", err)
	}
}
