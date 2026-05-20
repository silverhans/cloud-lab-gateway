// Package asynq implements the queue ports (TaskQueue, TaskRegistry, EventBus)
// on top of Redis: durable async tasks via hibiken/asynq, fan-out events via
// Redis pub/sub.
package asynq

import (
	"github.com/hibiken/asynq"

	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

// knownQueues lists every asynq queue the system uses, highest priority first.
// Cancel iterates them because asynq's Inspector requires an explicit queue.
var knownQueues = []string{"deploy", "cleanup", "checks", "default"}

// queuePriorities is the weight map handed to the asynq server. Higher weight
// means the scheduler pulls from that queue more often.
var queuePriorities = map[string]int{
	"deploy":  4,
	"cleanup": 2,
	"checks":  2,
	"default": 1,
}

// queueOf resolves the target queue for a task. An explicit Task.Queue wins;
// otherwise the queue is derived from the task type.
func queueOf(t ports.Task) string {
	if t.Queue != "" {
		return t.Queue
	}
	switch t.Type {
	case ports.TaskDeployLab:
		return "deploy"
	case ports.TaskCleanupLab, ports.TaskUnfreezeLab:
		return "cleanup"
	case ports.TaskRunCheck:
		return "checks"
	case ports.TaskRefreshQuota:
		return "default"
	default:
		return "default"
	}
}

// toAsynqTask converts a domain task into an asynq task.
func toAsynqTask(t ports.Task) *asynq.Task {
	return asynq.NewTask(string(t.Type), t.Payload)
}

// fromAsynqTask converts an asynq task back into a domain task. Only Type and
// Payload survive the round-trip — the other fields are enqueue-time options
// that handlers do not need.
func fromAsynqTask(t *asynq.Task) ports.Task {
	return ports.Task{
		Type:    ports.TaskType(t.Type()),
		Payload: t.Payload(),
	}
}

// enqueueOpts builds the asynq option list for a task.
func enqueueOpts(t ports.Task) []asynq.Option {
	opts := []asynq.Option{asynq.Queue(queueOf(t))}
	if t.MaxRetries > 0 {
		opts = append(opts, asynq.MaxRetry(t.MaxRetries))
	}
	if t.IdempotencyKey != "" {
		opts = append(opts, asynq.TaskID(t.IdempotencyKey))
	}
	return opts
}
