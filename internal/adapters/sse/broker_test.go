package sse

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/google/uuid"
)

func TestBrokerProjectsLabStateChangedToOwnerAudience(t *testing.T) {
	t.Parallel()

	ownerID := shared.UserID(uuid.New())
	labID := shared.LabInstanceID(uuid.New())
	b := NewBroker(nil, LabResolverFunc(func(context.Context, shared.LabInstanceID) (shared.UserID, error) {
		return ownerID, nil
	}), nil)

	payload := []byte(`{"lab_id":"` + labID.String() + `","to":"ready","reason":"deploy_succeeded"}`)
	event, audience, err := b.project(context.Background(), "lab.state_changed", payload)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if audience != "user:"+ownerID.String() {
		t.Fatalf("audience = %q", audience)
	}
	var data map[string]string
	if err := json.Unmarshal(event.Data, &data); err != nil {
		t.Fatalf("event data json: %v", err)
	}
	if data["type"] != "lab.state_changed" || data["labId"] != labID.String() || data["state"] != "ready" {
		t.Fatalf("unexpected data: %#v", data)
	}
}

func TestBrokerSubscribePublishesMatchingAudience(t *testing.T) {
	t.Parallel()

	b := NewBroker(nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var buf safeBuffer
	done := make(chan error, 1)
	go func() {
		done <- b.Subscribe(ctx, []string{"user:1"}, &buf)
	}()
	waitFor(t, func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		return len(b.clients) == 1
	})

	b.Publish("user:2", ports.SSEEvent{Type: "lab.state_changed", Data: []byte(`{"ignored":true}`)})
	b.Publish("user:1", ports.SSEEvent{Type: "lab.state_changed", Data: []byte(`{"type":"lab.state_changed"}`)})

	waitFor(t, func() bool { return strings.Contains(buf.String(), `"lab.state_changed"`) })
	if strings.Contains(buf.String(), "ignored") {
		t.Fatalf("received event for another audience: %s", buf.String())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("subscribe did not exit after context cancel")
	}
	waitFor(t, func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		return len(b.clients) == 0
	})
}

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestBrokerDropsSlowClient(t *testing.T) {
	t.Parallel()

	b := NewBroker(nil, nil, nil)
	c := &client{
		events:    make(chan ports.SSEEvent, 1),
		done:      make(chan struct{}),
		audiences: []string{"user:slow"},
	}
	c.events <- ports.SSEEvent{Type: "lab.state_changed", Data: []byte(`{}`)}
	b.add(c)

	b.Publish("user:slow", ports.SSEEvent{Type: "lab.state_changed", Data: []byte(`{}`)})

	b.mu.Lock()
	_, stillRegistered := b.clients[c]
	b.mu.Unlock()
	if stillRegistered {
		t.Fatal("slow client was not dropped")
	}
	select {
	case <-c.done:
	default:
		t.Fatal("slow client done channel was not closed")
	}
}

func waitFor(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met")
}
