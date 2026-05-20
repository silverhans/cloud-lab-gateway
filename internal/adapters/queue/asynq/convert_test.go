package asynq

import (
	"bytes"
	"testing"

	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

func TestQueueOf_ByType(t *testing.T) {
	t.Parallel()
	cases := map[ports.TaskType]string{
		ports.TaskDeployLab:    "deploy",
		ports.TaskCleanupLab:   "cleanup",
		ports.TaskUnfreezeLab:  "cleanup",
		ports.TaskRunCheck:     "checks",
		ports.TaskRefreshQuota: "default",
		ports.TaskType("???"):  "default",
	}
	for typ, want := range cases {
		typ, want := typ, want
		t.Run(string(typ), func(t *testing.T) {
			t.Parallel()
			if got := queueOf(ports.Task{Type: typ}); got != want {
				t.Errorf("queueOf(%s) = %s, want %s", typ, got, want)
			}
		})
	}
}

func TestQueueOf_ExplicitQueueWins(t *testing.T) {
	t.Parallel()
	got := queueOf(ports.Task{Type: ports.TaskDeployLab, Queue: "custom"})
	if got != "custom" {
		t.Errorf("explicit queue ignored: got %s", got)
	}
}

func TestEnqueueOpts(t *testing.T) {
	t.Parallel()

	// Minimal task: only the queue option.
	if n := len(enqueueOpts(ports.Task{Type: ports.TaskDeployLab})); n != 1 {
		t.Errorf("bare task: expected 1 opt (queue), got %d", n)
	}

	// Full task: queue + max-retry + task-id.
	full := ports.Task{
		Type:           ports.TaskDeployLab,
		MaxRetries:     5,
		IdempotencyKey: "deploy:abc:1",
	}
	if n := len(enqueueOpts(full)); n != 3 {
		t.Errorf("full task: expected 3 opts, got %d", n)
	}
}

func TestToFromAsynqTask_RoundTrip(t *testing.T) {
	t.Parallel()
	original := ports.Task{
		Type:    ports.TaskRunCheck,
		Payload: []byte(`{"lab_id":"x"}`),
	}
	got := fromAsynqTask(toAsynqTask(original))
	if got.Type != original.Type {
		t.Errorf("type round-trip: got %s, want %s", got.Type, original.Type)
	}
	if !bytes.Equal(got.Payload, original.Payload) {
		t.Errorf("payload round-trip: got %q, want %q", got.Payload, original.Payload)
	}
}

func TestKnownQueuesCoverPriorities(t *testing.T) {
	t.Parallel()
	for _, q := range knownQueues {
		if _, ok := queuePriorities[q]; !ok {
			t.Errorf("queue %q is in knownQueues but missing from queuePriorities", q)
		}
	}
	if len(knownQueues) != len(queuePriorities) {
		t.Errorf("knownQueues (%d) and queuePriorities (%d) are out of sync",
			len(knownQueues), len(queuePriorities))
	}
}
