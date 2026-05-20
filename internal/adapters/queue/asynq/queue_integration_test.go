//go:build integration

package asynq

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

func redisAddr(t *testing.T) string {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR not set")
	}
	return addr
}

func TestClient_EnqueueAndCancel(t *testing.T) {
	addr := redisAddr(t)
	c := NewClient(addr)
	defer func() { _ = c.Close() }()

	key := "deploy:" + uuid.NewString() + ":1"
	id, err := c.EnqueueAt(context.Background(), ports.Task{
		Type:           ports.TaskDeployLab,
		Payload:        []byte(`{}`),
		IdempotencyKey: key,
	}, time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("enqueue-at: %v", err)
	}
	if id != key {
		t.Errorf("expected id == idempotency key, got %s", id)
	}

	// Cancelling a scheduled task succeeds.
	if err := c.Cancel(context.Background(), key); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	// Cancelling again is a no-op (idempotent).
	if err := c.Cancel(context.Background(), key); err != nil {
		t.Fatalf("second cancel should be a no-op: %v", err)
	}
}

func TestClient_IdempotentEnqueue(t *testing.T) {
	addr := redisAddr(t)
	c := NewClient(addr)
	defer func() { _ = c.Close() }()

	key := "check:" + uuid.NewString() + ":1"
	task := ports.Task{Type: ports.TaskRunCheck, Payload: []byte(`{}`), IdempotencyKey: key}

	id1, err := c.Enqueue(context.Background(), task)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	id2, err := c.Enqueue(context.Background(), task)
	if err != nil {
		t.Fatalf("second enqueue must not error (idempotent): %v", err)
	}
	if id1 != id2 {
		t.Errorf("idempotent enqueue returned different ids: %s vs %s", id1, id2)
	}
	_ = c.Cancel(context.Background(), key)
}

func TestEventBus_PublishSubscribe(t *testing.T) {
	addr := redisAddr(t)
	bus := NewEventBus(addr)
	defer func() { _ = bus.Close() }()

	topic := "test.topic." + uuid.NewString()
	ch, cancel, err := bus.Subscribe(topic)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer cancel()

	// Give the subscription a moment to register with Redis.
	time.Sleep(100 * time.Millisecond)

	want := []byte(`{"event":"hello"}`)
	if err := bus.Publish(context.Background(), topic, want); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case msg := <-ch:
		if msg.Topic != topic {
			t.Errorf("topic = %s, want %s", msg.Topic, topic)
		}
		if string(msg.Payload) != string(want) {
			t.Errorf("payload = %s, want %s", msg.Payload, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive published message within 2s")
	}
}

func TestRegistry_RoundTrip(t *testing.T) {
	addr := redisAddr(t)
	client := NewClient(addr)
	defer func() { _ = client.Close() }()

	registry := NewRegistry(addr, 4, nil)

	received := make(chan ports.Task, 1)
	if err := registry.Subscribe(ports.TaskRefreshQuota, func(_ context.Context, task ports.Task) error {
		received <- task
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = registry.Run(ctx) }()
	time.Sleep(300 * time.Millisecond) // let the worker start

	payload := []byte(`{"refresh":true}`)
	if _, err := client.Enqueue(context.Background(), ports.Task{
		Type:    ports.TaskRefreshQuota,
		Payload: payload,
	}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	select {
	case got := <-received:
		if got.Type != ports.TaskRefreshQuota {
			t.Errorf("handler got type %s", got.Type)
		}
		if string(got.Payload) != string(payload) {
			t.Errorf("handler got payload %s, want %s", got.Payload, payload)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not run within 5s")
	}
}
