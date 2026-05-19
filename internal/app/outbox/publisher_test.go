package outbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

// fakeRepo is an in-memory Repo for unit tests.
type fakeRepo struct {
	mu          sync.Mutex
	rows        []Row
	published   map[int64]bool
	fetchErr    error
	bumpErr     error
	publishedAt map[int64]time.Time
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		published:   make(map[int64]bool),
		publishedAt: make(map[int64]time.Time),
	}
}

func (r *fakeRepo) add(rows ...Row) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, rows...)
}

func (r *fakeRepo) Fetch(_ context.Context, limit, maxAttempts int) ([]Row, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fetchErr != nil {
		return nil, r.fetchErr
	}
	out := make([]Row, 0, limit)
	for _, row := range r.rows {
		if r.published[row.ID] {
			continue
		}
		if row.Attempts >= maxAttempts {
			continue
		}
		out = append(out, row)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *fakeRepo) MarkPublished(_ context.Context, ids []int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for _, id := range ids {
		r.published[id] = true
		r.publishedAt[id] = now
	}
	return nil
}

func (r *fakeRepo) BumpAttempts(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.bumpErr != nil {
		return r.bumpErr
	}
	for i := range r.rows {
		if r.rows[i].ID == id {
			r.rows[i].Attempts++
			return nil
		}
	}
	return nil
}

func (r *fakeRepo) publishedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.published)
}

func (r *fakeRepo) attemptsFor(id int64) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.rows {
		if row.ID == id {
			return row.Attempts
		}
	}
	return -1
}

// fakeBus captures publishes; supports a per-topic failure injection.
type fakeBus struct {
	mu       sync.Mutex
	got      []ports.Message
	failOn   map[string]error // topic → error to return
	delay    time.Duration
}

func newFakeBus() *fakeBus { return &fakeBus{failOn: map[string]error{}} }

func (b *fakeBus) Publish(_ context.Context, topic string, payload []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err, ok := b.failOn[topic]; ok {
		return err
	}
	if b.delay > 0 {
		time.Sleep(b.delay)
	}
	b.got = append(b.got, ports.Message{Topic: topic, Payload: append([]byte(nil), payload...)})
	return nil
}

func (b *fakeBus) Subscribe(string) (<-chan ports.Message, func(), error) {
	// Not exercised by publisher tests.
	ch := make(chan ports.Message)
	return ch, func() { close(ch) }, nil
}

func (b *fakeBus) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.got)
}

func (b *fakeBus) topics() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.got))
	for i, m := range b.got {
		out[i] = m.Topic
	}
	return out
}

// ───────────────────────────────────────────────────────────────────────────

func TestTick_PublishesAllAndMarksThem(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	bus := newFakeBus()
	p := New(repo, bus, nil)

	repo.add(
		Row{ID: 1, Topic: "lab.state_changed", Payload: []byte(`{"a":1}`)},
		Row{ID: 2, Topic: "project.allocated", Payload: []byte(`{"b":2}`)},
		Row{ID: 3, Topic: "quota.blocked", Payload: []byte(`{"c":3}`)},
	)

	if err := p.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if got := repo.publishedCount(); got != 3 {
		t.Errorf("expected 3 marked published, got %d", got)
	}
	if got := bus.count(); got != 3 {
		t.Errorf("expected 3 messages on bus, got %d", got)
	}
	want := []string{"lab.state_changed", "project.allocated", "quota.blocked"}
	for i, topic := range bus.topics() {
		if topic != want[i] {
			t.Errorf("bus[%d] = %s, want %s", i, topic, want[i])
		}
	}
}

func TestTick_BusFailureBumpsAttempts(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	bus := newFakeBus()
	bus.failOn["lab.state_changed"] = errors.New("redis down")
	p := New(repo, bus, nil)

	repo.add(
		Row{ID: 10, Topic: "lab.state_changed", Payload: []byte(`{}`)},
		Row{ID: 11, Topic: "quota.blocked", Payload: []byte(`{}`)},
	)

	if err := p.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	// quota.blocked should have published, lab.state_changed should not have.
	if got := repo.publishedCount(); got != 1 {
		t.Errorf("expected 1 published (quota.blocked), got %d", got)
	}
	if got := repo.attemptsFor(10); got != 1 {
		t.Errorf("expected attempts=1 for failed row, got %d", got)
	}
}

func TestTick_PoisonMessagesAreSkippedAfterMaxAttempts(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	bus := newFakeBus()
	p := New(repo, bus, nil)
	p.MaxAttempts = 3

	repo.add(
		Row{ID: 20, Topic: "lab.state_changed", Payload: []byte(`{}`), Attempts: 3},
		Row{ID: 21, Topic: "quota.blocked", Payload: []byte(`{}`), Attempts: 0},
	)

	if err := p.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if got := bus.count(); got != 1 {
		t.Errorf("expected only the non-poison message on bus, got %d", got)
	}
	if got := repo.publishedCount(); got != 1 {
		t.Errorf("expected 1 row marked published, got %d", got)
	}
}

func TestTick_EmptyOutboxIsNoop(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	bus := newFakeBus()
	p := New(repo, bus, nil)
	if err := p.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if bus.count() != 0 || repo.publishedCount() != 0 {
		t.Errorf("expected no side effects on empty outbox")
	}
}

func TestTick_RepoFetchErrorPropagates(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	wantErr := errors.New("postgres down")
	repo.fetchErr = wantErr
	p := New(repo, newFakeBus(), nil)
	if err := p.Tick(context.Background()); !errors.Is(err, wantErr) {
		t.Errorf("expected fetch error to propagate, got %v", err)
	}
}

func TestRun_StopsOnContextCancel(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	bus := newFakeBus()
	p := New(repo, bus, nil)
	p.PollInterval = 20 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	time.Sleep(60 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("publisher did not stop within 1s of cancel")
	}
}

func TestRun_FlushesAddedRowsOverTime(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	bus := newFakeBus()
	p := New(repo, bus, nil)
	p.PollInterval = 15 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = p.Run(ctx) }()

	// First batch — should be picked up on the immediate tick.
	repo.add(Row{ID: 100, Topic: "lab.state_changed", Payload: []byte(`{}`)})
	// Second batch arrives a tick later.
	time.Sleep(25 * time.Millisecond)
	repo.add(Row{ID: 101, Topic: "lab.state_changed", Payload: []byte(`{}`)})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if bus.count() >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if bus.count() != 2 {
		t.Errorf("expected 2 published messages within 500ms, got %d", bus.count())
	}
}
