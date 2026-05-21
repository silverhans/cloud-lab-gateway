package pgxrepo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloud-lab-gateway/gateway/internal/adapters/storage/sqlcgen"
	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/domain/verify"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CheckRunRepo persists verify.CheckRun aggregates in Postgres.
type CheckRunRepo struct {
	q *sqlcgen.Queries
}

var _ ports.CheckRunRepo = (*CheckRunRepo)(nil)

// NewCheckRunRepo creates a CheckRunRepo backed by a pgx pool.
func NewCheckRunRepo(db *pgxpool.Pool) *CheckRunRepo {
	return &CheckRunRepo{q: sqlcgen.New(db)}
}

func (r *CheckRunRepo) Create(ctx context.Context, tx ports.Tx, run *verify.CheckRun) error {
	if run == nil {
		return shared.ErrInvalidInput
	}
	q, err := r.queriesInTx(tx)
	if err != nil {
		return err
	}
	if err := q.CreateCheckRun(ctx, checkRunParams(run)); err != nil {
		return fmt.Errorf("pgxrepo: create check run: %w", err)
	}
	return replaceCheckRunSteps(ctx, q, run)
}

func (r *CheckRunRepo) Save(ctx context.Context, tx ports.Tx, run *verify.CheckRun) error {
	if run == nil {
		return shared.ErrInvalidInput
	}
	q, err := r.queriesInTx(tx)
	if err != nil {
		return err
	}
	if err := q.UpdateCheckRun(ctx, updateCheckRunParams(run)); err != nil {
		return fmt.Errorf("pgxrepo: save check run: %w", err)
	}
	return replaceCheckRunSteps(ctx, q, run)
}

func (r *CheckRunRepo) GetByID(ctx context.Context, id shared.CheckRunID) (*verify.CheckRun, error) {
	row, err := r.q.GetCheckRunByID(ctx, checkRunID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: get check run: %w", err)
	}
	run := checkRunFromRow(row)
	steps, err := r.q.ListCheckRunSteps(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: list check run steps: %w", err)
	}
	run.Steps, err = stepResultsFromRows(steps)
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *CheckRunRepo) ListByLab(ctx context.Context, labIDValue shared.LabInstanceID, limit int) ([]verify.CheckRun, error) {
	limit32, err := listLimitInt32(limit)
	if err != nil {
		return nil, err
	}
	if limit32 == 0 {
		limit32 = 50
	}
	rows, err := r.q.ListCheckRunsByLab(ctx, sqlcgen.ListCheckRunsByLabParams{
		LabInstanceID: labID(labIDValue),
		Limit:         limit32,
	})
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: list check runs: %w", err)
	}
	out := make([]verify.CheckRun, 0, len(rows))
	for _, row := range rows {
		out = append(out, checkRunFromRow(row))
	}
	return out, nil
}

func (r *CheckRunRepo) queriesInTx(tx ports.Tx) (*sqlcgen.Queries, error) {
	pgxTx, err := txFromPort(tx)
	if err != nil {
		return nil, err
	}
	return r.q.WithTx(pgxTx), nil
}

func replaceCheckRunSteps(ctx context.Context, q *sqlcgen.Queries, run *verify.CheckRun) error {
	if err := q.DeleteCheckRunSteps(ctx, checkRunID(run.ID)); err != nil {
		return fmt.Errorf("pgxrepo: delete check run steps: %w", err)
	}
	for i, step := range run.Steps {
		params, err := checkRunStepParams(run.ID, i, step)
		if err != nil {
			return err
		}
		if err := q.InsertCheckRunStep(ctx, params); err != nil {
			return fmt.Errorf("pgxrepo: insert check run step: %w", err)
		}
	}
	return nil
}

func checkRunParams(run *verify.CheckRun) sqlcgen.CreateCheckRunParams {
	return sqlcgen.CreateCheckRunParams{
		ID:                checkRunID(run.ID),
		LabInstanceID:     labID(run.LabInstanceID),
		CheckTemplateID:   checkTemplateID(run.CheckTemplateID),
		TriggeredByUserID: nullableUserID(run.TriggeredBy),
		State:             string(run.State),
		Summary:           run.Summary,
		AnsibleStdout:     run.AnsibleStdout,
		StartedAt:         timestamptzPtr(run.StartedAt),
		FinishedAt:        timestamptzPtr(run.FinishedAt),
	}
}

func updateCheckRunParams(run *verify.CheckRun) sqlcgen.UpdateCheckRunParams {
	return sqlcgen.UpdateCheckRunParams{
		ID:            checkRunID(run.ID),
		State:         string(run.State),
		Summary:       run.Summary,
		AnsibleStdout: run.AnsibleStdout,
		StartedAt:     timestamptzPtr(run.StartedAt),
		FinishedAt:    timestamptzPtr(run.FinishedAt),
	}
}

func checkRunFromRow(row sqlcgen.CheckRun) verify.CheckRun {
	return verify.CheckRun{
		ID:              shared.CheckRunID(row.ID),
		LabInstanceID:   shared.LabInstanceID(row.LabInstanceID),
		CheckTemplateID: shared.CheckTemplate(row.CheckTemplateID),
		TriggeredBy:     userIDFromNull(row.TriggeredByUserID),
		State:           verify.RunState(row.State),
		StartedAt:       timePtrFromTimestamptz(row.StartedAt),
		FinishedAt:      timePtrFromTimestamptz(row.FinishedAt),
		Summary:         row.Summary,
		AnsibleStdout:   row.AnsibleStdout,
	}
}

func checkRunStepParams(runID shared.CheckRunID, idx int, step verify.StepResult) (sqlcgen.InsertCheckRunStepParams, error) {
	expected, err := jsonBytes(step.Expected)
	if err != nil {
		return sqlcgen.InsertCheckRunStepParams{}, err
	}
	actual, err := jsonBytes(step.Actual)
	if err != nil {
		return sqlcgen.InsertCheckRunStepParams{}, err
	}
	return sqlcgen.InsertCheckRunStepParams{
		CheckRunID: checkRunID(runID),
		Seq:        int32(idx + 1),
		TaskName:   step.TaskName,
		Status:     string(step.Status),
		Expected:   expected,
		Actual:     actual,
		Message:    step.Message,
	}, nil
}

func stepResultsFromRows(rows []sqlcgen.CheckRunStep) ([]verify.StepResult, error) {
	out := make([]verify.StepResult, 0, len(rows))
	for _, row := range rows {
		expected, err := jsonValue(row.Expected)
		if err != nil {
			return nil, err
		}
		actual, err := jsonValue(row.Actual)
		if err != nil {
			return nil, err
		}
		msg := ""
		if row.Message != nil {
			msg = *row.Message
		}
		out = append(out, verify.StepResult{
			TaskName: row.TaskName,
			Status:   verify.StepStatus(row.Status),
			Expected: expected,
			Actual:   actual,
			Message:  msg,
		})
	}
	return out, nil
}

func jsonBytes(v interface{}) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	out, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("pgxrepo: marshal check step json: %w", err)
	}
	return out, nil
}

func jsonValue(raw []byte) (interface{}, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var out interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("pgxrepo: unmarshal check step json: %w", err)
	}
	return out, nil
}

func checkRunID(id shared.CheckRunID) uuid.UUID {
	if id == (shared.CheckRunID{}) {
		return uuid.New()
	}
	return uuid.UUID(id)
}

func checkTemplateID(id shared.CheckTemplate) uuid.UUID {
	return uuid.UUID(id)
}
