//go:build integration

package pgxrepo

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/pool"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/google/uuid"
)

func TestAllocateConcurrent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	repo := NewPoolRepo(db)
	uow := NewUoW(db)
	domainID := "test-domain-" + uuid.NewString()

	projects := make([]pool.Project, 10)
	for i := range projects {
		projects[i] = pool.Project{
			ID:          shared.NewProjectID(),
			KIProjectID: "ki-" + uuid.NewString(),
			KIDomainID:  domainID,
			Name:        "slot-" + uuid.NewString(),
			State:       pool.StateFree,
		}
	}
	if err := repo.SeedInsert(ctx, projects); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	allocated := map[string]bool{}
	emptyCount := 0
	errCount := 0
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			labID := shared.NewLabInstanceID()
			var projectID string
			err := uow.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
				project, err := repo.AllocateOneFree(ctx, tx, domainID, labID)
				if err != nil {
					return err
				}
				projectID = project.ID.String()
				return nil
			})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				if allocated[projectID] {
					errCount++
				}
				allocated[projectID] = true
			case errors.Is(err, shared.ErrPoolEmpty):
				emptyCount++
			default:
				errCount++
			}
		}()
	}
	wg.Wait()

	if len(allocated) != 10 || emptyCount != 40 || errCount != 0 {
		t.Fatalf("allocated=%d empty=%d err=%d", len(allocated), emptyCount, errCount)
	}
}

func TestSaveFlushesEventsToOutbox(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	repo := NewPoolRepo(db)
	uow := NewUoW(db)
	domainID := "test-domain-" + uuid.NewString()

	if err := repo.SeedInsert(ctx, []pool.Project{{
		ID:          shared.NewProjectID(),
		KIProjectID: "ki-" + uuid.NewString(),
		KIDomainID:  domainID,
		Name:        "slot-" + uuid.NewString(),
		State:       pool.StateFree,
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var project *pool.Project
	if err := uow.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		var err error
		project, err = repo.AllocateOneFree(ctx, tx, domainID, shared.NewLabInstanceID())
		return err
	}); err != nil {
		t.Fatalf("allocate: %v", err)
	}
	project.PullEvents()

	if err := project.Release(time.Now().UTC().Add(time.Minute)); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := uow.WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		return repo.Save(ctx, tx, project)
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	var count int
	err := db.QueryRow(ctx, "SELECT count(*) FROM outbox WHERE topic = 'project.releasing' AND payload->>'project_id' = $1", project.ID.String()).Scan(&count)
	if err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one outbox event, got %d", count)
	}
}
