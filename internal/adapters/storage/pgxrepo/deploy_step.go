package pgxrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloud-lab-gateway/gateway/internal/adapters/storage/sqlcgen"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DeployStepRepo persists deploy-saga step records in Postgres.
type DeployStepRepo struct {
	q *sqlcgen.Queries
}

var _ ports.DeployStepRepo = (*DeployStepRepo)(nil)

// NewDeployStepRepo creates a DeployStepRepo backed by a pgx pool.
func NewDeployStepRepo(db *pgxpool.Pool) *DeployStepRepo {
	return &DeployStepRepo{q: sqlcgen.New(db)}
}

// GetOrInit returns an existing step record or a fresh pending value.
func (r *DeployStepRepo) GetOrInit(ctx context.Context, labIDValue shared.LabInstanceID, step string) (ports.DeployStep, error) {
	row, err := r.q.GetDeployStep(ctx, sqlcgen.GetDeployStepParams{
		LabInstanceID: labID(labIDValue),
		StepName:      step,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.DeployStep{
			LabID:    labIDValue,
			StepName: step,
			Status:   "pending",
			Attempt:  0,
		}, nil
	}
	if err != nil {
		return ports.DeployStep{}, fmt.Errorf("pgxrepo: get deploy step: %w", err)
	}
	return deployStepFromRow(row), nil
}

// Save inserts or updates one deploy-step record.
func (r *DeployStepRepo) Save(ctx context.Context, s ports.DeployStep) error {
	attempt, err := deployStepAttemptInt32(s.Attempt)
	if err != nil {
		return err
	}
	if err := r.q.UpsertDeployStep(ctx, sqlcgen.UpsertDeployStepParams{
		LabInstanceID: labID(s.LabID),
		StepName:      s.StepName,
		Status:        s.Status,
		Attempt:       attempt,
		Column5:       s.LastError,
		Result:        nullableJSONBytes(s.Result),
		StartedAt:     timestamptzPtr(s.StartedAt),
		FinishedAt:    timestamptzPtr(s.FinishedAt),
	}); err != nil {
		return fmt.Errorf("pgxrepo: save deploy step: %w", err)
	}
	return nil
}

// ListByLab returns all deploy steps for a lab in deterministic order.
func (r *DeployStepRepo) ListByLab(ctx context.Context, labIDValue shared.LabInstanceID) ([]ports.DeployStep, error) {
	rows, err := r.q.ListDeployStepsByLab(ctx, labID(labIDValue))
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: list deploy steps: %w", err)
	}
	out := make([]ports.DeployStep, 0, len(rows))
	for _, row := range rows {
		out = append(out, deployStepFromRow(row))
	}
	return out, nil
}

func deployStepFromRow(row sqlcgen.LabDeployStep) ports.DeployStep {
	lastError := ""
	if row.LastError != nil {
		lastError = *row.LastError
	}
	return ports.DeployStep{
		LabID:      shared.LabInstanceID(row.LabInstanceID),
		StepName:   row.StepName,
		Status:     row.Status,
		Attempt:    int(row.Attempt),
		LastError:  lastError,
		Result:     nullableBytesFromRow(row.Result),
		StartedAt:  timePtrFromTimestamptz(row.StartedAt),
		FinishedAt: timePtrFromTimestamptz(row.FinishedAt),
	}
}

func deployStepAttemptInt32(n int) (int32, error) {
	const maxInt32 = int(^uint32(0) >> 1)
	if n < 0 || n > maxInt32 {
		return 0, shared.ErrInvalidInput
	}
	return int32(n), nil
}

func nullableJSONBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}

func nullableBytesFromRow(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return append([]byte(nil), b...)
}
