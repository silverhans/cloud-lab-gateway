package ports

import (
	"context"
	"time"
)

// TaskQueue dispatches durable async work to workers. The default adapter is
// asynq on Redis; tests use an in-memory implementation.
type TaskQueue interface {
	// Enqueue schedules a task. Returns the task ID. Idempotent if IdempotencyKey is set.
	Enqueue(ctx context.Context, t Task) (string, error)

	// EnqueueAt schedules a task to run at a specific time. Used for cleanup/unfreeze timers.
	EnqueueAt(ctx context.Context, t Task, runAt time.Time) (string, error)

	// Cancel removes a scheduled task if not yet started.
	Cancel(ctx context.Context, taskID string) error
}

// Task is a unit of async work. Type is used to route to the right handler.
type Task struct {
	Type           TaskType
	Payload        []byte // JSON
	IdempotencyKey string
	Queue          string // optional: "default", "deploy", "cleanup", "checks"
	MaxRetries     int
}

type TaskType string

const (
	TaskDeployLab    TaskType = "deploy_lab"
	TaskCleanupLab   TaskType = "cleanup_lab"
	TaskRunCheck     TaskType = "run_check"
	TaskRefreshQuota TaskType = "refresh_quota"
	TaskUnfreezeLab  TaskType = "unfreeze_lab_expired"
)

// TaskHandler is implemented by application-layer code for each TaskType.
type TaskHandler func(ctx context.Context, t Task) error

// TaskRegistry binds handlers to task types. Workers call Subscribe at start.
type TaskRegistry interface {
	Subscribe(typ TaskType, handler TaskHandler) error
	Run(ctx context.Context) error // blocking
}

// EventBus distributes domain events to in-process subscribers (e.g. the SSE
// broker). The transactional outbox writes to AuditRepo first; a publisher
// then forwards events here.
type EventBus interface {
	Publish(ctx context.Context, topic string, payload []byte) error
	Subscribe(topic string) (<-chan Message, func(), error)
}

type Message struct {
	Topic   string
	Payload []byte
}
