//go:build integration

package pgxrepo

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	labdomain "github.com/cloud-lab-gateway/gateway/internal/domain/lab"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDeployStepRepoGetOrInitUnknown(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := NewDeployStepRepo(connectTestDB(t))
	labID := shared.NewLabInstanceID()

	got, err := repo.GetOrInit(ctx, labID, "create_keypair")
	if err != nil {
		t.Fatalf("GetOrInit: %v", err)
	}
	if got.LabID != labID || got.StepName != "create_keypair" || got.Status != "pending" || got.Attempt != 0 {
		t.Fatalf("unexpected fresh step: %+v", got)
	}
	if got.StartedAt != nil || got.FinishedAt != nil || got.Result != nil {
		t.Fatalf("fresh step should have nil nullable fields: %+v", got)
	}
}

func TestDeployStepRepoSaveThenGetRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	repo := NewDeployStepRepo(db)
	labID := insertDeployStepLab(t, db)
	started := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	finished := started.Add(time.Minute)
	want := ports.DeployStep{
		LabID:      labID,
		StepName:   "boot_vm",
		Status:     "failed",
		Attempt:    2,
		LastError:  "boom",
		Result:     []byte(`{"server_ids":["srv-1"]}`),
		StartedAt:  &started,
		FinishedAt: &finished,
	}

	if err := repo.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.GetOrInit(ctx, labID, "boot_vm")
	if err != nil {
		t.Fatalf("GetOrInit: %v", err)
	}
	if got.LabID != want.LabID || got.StepName != want.StepName || got.Status != want.Status ||
		got.Attempt != want.Attempt || got.LastError != want.LastError ||
		!jsonEqual(got.Result, want.Result) || got.StartedAt == nil || got.FinishedAt == nil ||
		!got.StartedAt.Equal(started) || !got.FinishedAt.Equal(finished) {
		t.Fatalf("round trip mismatch:\ngot  %+v\nwant %+v", got, want)
	}
}

func TestDeployStepRepoSaveTwiceUpserts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	repo := NewDeployStepRepo(db)
	labID := insertDeployStepLab(t, db)

	if err := repo.Save(ctx, ports.DeployStep{LabID: labID, StepName: "wait_ssh", Status: "in_progress", Attempt: 1}); err != nil {
		t.Fatalf("Save first: %v", err)
	}
	if err := repo.Save(ctx, ports.DeployStep{LabID: labID, StepName: "wait_ssh", Status: "succeeded", Attempt: 2, Result: []byte(`{"ok":true}`)}); err != nil {
		t.Fatalf("Save second: %v", err)
	}

	steps, err := repo.ListByLab(ctx, labID)
	if err != nil {
		t.Fatalf("ListByLab: %v", err)
	}
	if len(steps) != 1 || steps[0].Status != "succeeded" || steps[0].Attempt != 2 {
		t.Fatalf("expected one updated step, got %+v", steps)
	}
}

func TestDeployStepRepoListByLab(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := connectTestDB(t)
	repo := NewDeployStepRepo(db)
	labID := insertDeployStepLab(t, db)
	emptyLabID := insertDeployStepLab(t, db)

	for _, step := range []string{"create_keypair", "provision_network", "boot_vm"} {
		if err := repo.Save(ctx, ports.DeployStep{LabID: labID, StepName: step, Status: "succeeded", Attempt: 1}); err != nil {
			t.Fatalf("Save %s: %v", step, err)
		}
	}

	steps, err := repo.ListByLab(ctx, labID)
	if err != nil {
		t.Fatalf("ListByLab: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %+v", steps)
	}
	empty, err := repo.ListByLab(ctx, emptyLabID)
	if err != nil {
		t.Fatalf("ListByLab empty: %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("expected non-nil empty slice, got %#v", empty)
	}
}

func insertDeployStepLab(t *testing.T, db *pgxpool.Pool) shared.LabInstanceID {
	t.Helper()
	ctx := context.Background()
	refs := seedLabRefs(t, db)
	inst := labdomain.New(shared.NewLabInstanceID(), refs.StudentID, refs.CourseID, refs.TemplateID, time.Now().UTC())
	if err := NewUoW(db).WithTx(ctx, func(ctx context.Context, tx ports.Tx) error {
		return NewLabRepo(db).Create(ctx, tx, inst)
	}); err != nil {
		t.Fatalf("create parent lab: %v", err)
	}
	return inst.ID
}

func jsonEqual(a, b []byte) bool {
	var av interface{}
	var bv interface{}
	if err := json.Unmarshal(a, &av); err != nil {
		return bytes.Equal(a, b)
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return bytes.Equal(a, b)
	}
	return reflect.DeepEqual(av, bv)
}
