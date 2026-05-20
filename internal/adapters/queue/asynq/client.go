package asynq

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"

	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

// Client implements ports.TaskQueue: it enqueues durable tasks and cancels
// scheduled ones.
type Client struct {
	client    *asynq.Client
	inspector *asynq.Inspector
}

var _ ports.TaskQueue = (*Client)(nil)

// NewClient connects to Redis at addr.
func NewClient(redisAddr string) *Client {
	opt := asynq.RedisClientOpt{Addr: redisAddr}
	return &Client{
		client:    asynq.NewClient(opt),
		inspector: asynq.NewInspector(opt),
	}
}

// Close releases the underlying Redis connections.
func (c *Client) Close() error {
	return errors.Join(c.client.Close(), c.inspector.Close())
}

// Enqueue schedules a task for immediate processing.
func (c *Client) Enqueue(ctx context.Context, t ports.Task) (string, error) {
	return c.enqueue(ctx, t, enqueueOpts(t))
}

// EnqueueAt schedules a task to run at runAt — used for cleanup and unfreeze
// timers.
func (c *Client) EnqueueAt(ctx context.Context, t ports.Task, runAt time.Time) (string, error) {
	opts := append(enqueueOpts(t), asynq.ProcessAt(runAt))
	return c.enqueue(ctx, t, opts)
}

func (c *Client) enqueue(ctx context.Context, t ports.Task, opts []asynq.Option) (string, error) {
	info, err := c.client.EnqueueContext(ctx, toAsynqTask(t), opts...)
	if err != nil {
		// Idempotent enqueue: a task with the same TaskID already exists.
		// The caller asked for at-most-once delivery via IdempotencyKey, so a
		// conflict means "already done" — report success with the known ID.
		if t.IdempotencyKey != "" &&
			(errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask)) {
			return t.IdempotencyKey, nil
		}
		return "", fmt.Errorf("asynq: enqueue %s: %w", t.Type, err)
	}
	return info.ID, nil
}

// Cancel removes a scheduled or pending task. asynq's Inspector needs an
// explicit queue name, so we try every known queue. A task absent from all of
// them is treated as already gone (Cancel is idempotent).
func (c *Client) Cancel(_ context.Context, taskID string) error {
	if taskID == "" {
		return nil
	}
	for _, q := range knownQueues {
		err := c.inspector.DeleteTask(q, taskID)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, asynq.ErrTaskNotFound), errors.Is(err, asynq.ErrQueueNotFound):
			continue
		default:
			return fmt.Errorf("asynq: cancel %s in queue %s: %w", taskID, q, err)
		}
	}
	return nil
}
