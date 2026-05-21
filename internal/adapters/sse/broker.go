package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	audienceAll       = "all"
	clientBufferSize  = 32
	keepaliveInterval = 20 * time.Second
)

var eventTopics = []string{
	"lab.state_changed",
	"lab.deploy_progress",
	"check.state_changed",
	"quota.snapshot",
}

type LabResolver interface {
	ResolveLabOwner(ctx context.Context, labID shared.LabInstanceID) (shared.UserID, error)
}

type LabResolverFunc func(ctx context.Context, labID shared.LabInstanceID) (shared.UserID, error)

func (f LabResolverFunc) ResolveLabOwner(ctx context.Context, labID shared.LabInstanceID) (shared.UserID, error) {
	return f(ctx, labID)
}

// Broker fans out SSE events to connected clients.
type Broker struct {
	bus      ports.EventBus
	resolver LabResolver
	log      *zap.Logger

	mu      sync.Mutex
	clients map[*client]struct{}
	byAud   map[string]map[*client]struct{}
}

var _ ports.SSEBroker = (*Broker)(nil)

func NewBroker(bus ports.EventBus, resolver LabResolver, log *zap.Logger) *Broker {
	if log == nil {
		log = zap.NewNop()
	}
	return &Broker{
		bus:      bus,
		resolver: resolver,
		log:      log,
		clients:  map[*client]struct{}{},
		byAud:    map[string]map[*client]struct{}{},
	}
}

// Start subscribes to the EventBus and projects durable outbox events into SSE.
func (b *Broker) Start(ctx context.Context) error {
	for _, topic := range eventTopics {
		messages, cancel, err := b.bus.Subscribe(topic)
		if err != nil {
			return fmt.Errorf("sse: subscribe %s: %w", topic, err)
		}
		go func(topic string, messages <-chan ports.Message, cancel func()) {
			defer cancel()
			for {
				select {
				case <-ctx.Done():
					return
				case msg, ok := <-messages:
					if !ok {
						return
					}
					b.handleMessage(ctx, topic, msg.Payload)
				}
			}
		}(topic, messages, cancel)
	}
	return nil
}

func (b *Broker) Publish(audience string, event ports.SSEEvent) {
	b.mu.Lock()
	targets := make([]*client, 0, len(b.byAud[audience]))
	for c := range b.byAud[audience] {
		targets = append(targets, c)
	}
	b.mu.Unlock()

	for _, c := range targets {
		select {
		case c.events <- event:
		default:
			b.drop(c)
		}
	}
}

func (b *Broker) Subscribe(ctx context.Context, audiences []string, w io.Writer) error {
	c := &client{
		events:    make(chan ports.SSEEvent, clientBufferSize),
		done:      make(chan struct{}),
		audiences: audiences,
	}
	b.add(c)
	defer b.drop(c)

	flusher, _ := w.(http.Flusher)
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.done:
			return nil
		case <-ticker.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return err
			}
			if flusher != nil {
				flusher.Flush()
			}
		case event, ok := <-c.events:
			if !ok {
				return nil
			}
			if err := writeEvent(w, event); err != nil {
				return err
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

func (b *Broker) handleMessage(ctx context.Context, topic string, payload []byte) {
	projection, audience, err := b.project(ctx, topic, payload)
	if err != nil {
		b.log.Debug("sse projection dropped", zap.String("topic", topic), zap.Error(err))
		return
	}
	b.Publish(audience, projection)
}

func (b *Broker) project(ctx context.Context, topic string, payload []byte) (ports.SSEEvent, string, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return ports.SSEEvent{}, "", err
	}
	switch topic {
	case "lab.state_changed":
		labID, err := labIDFrom(raw)
		if err != nil {
			return ports.SSEEvent{}, "", err
		}
		state := stringField(raw, "state")
		if state == "" {
			state = stringField(raw, "to")
		}
		data, err := json.Marshal(map[string]interface{}{
			"type":   "lab.state_changed",
			"labId":  labID.String(),
			"state":  state,
			"reason": stringField(raw, "reason"),
		})
		if err != nil {
			return ports.SSEEvent{}, "", err
		}
		audience, err := b.labAudience(ctx, labID)
		if err != nil {
			return ports.SSEEvent{}, "", err
		}
		return ports.SSEEvent{Type: topic, Data: data}, audience, nil
	case "lab.deploy_progress":
		labID, err := labIDFrom(raw)
		if err != nil {
			return ports.SSEEvent{}, "", err
		}
		data, err := json.Marshal(map[string]interface{}{
			"type":   "lab.deploy_progress",
			"labId":  labID.String(),
			"step":   firstString(raw, "step", "step_name"),
			"status": stringField(raw, "status"),
		})
		if err != nil {
			return ports.SSEEvent{}, "", err
		}
		audience, err := b.labAudience(ctx, labID)
		if err != nil {
			return ports.SSEEvent{}, "", err
		}
		return ports.SSEEvent{Type: topic, Data: data}, audience, nil
	case "check.state_changed":
		labID, err := labIDFrom(raw)
		if err != nil {
			return ports.SSEEvent{}, "", err
		}
		data, err := json.Marshal(map[string]interface{}{
			"type":    "check.state_changed",
			"labId":   labID.String(),
			"checkId": firstString(raw, "check_id", "checkRunId"),
			"state":   stringField(raw, "state"),
		})
		if err != nil {
			return ports.SSEEvent{}, "", err
		}
		audience, err := b.labAudience(ctx, labID)
		if err != nil {
			return ports.SSEEvent{}, "", err
		}
		return ports.SSEEvent{Type: topic, Data: data}, audience, nil
	case "quota.snapshot":
		data, err := json.Marshal(map[string]interface{}{
			"type": "quota.snapshot",
			"cpu":  numberField(raw, "cpu", "vcpus"),
			"ram":  numberField(raw, "ram"),
			"disk": numberField(raw, "disk"),
		})
		if err != nil {
			return ports.SSEEvent{}, "", err
		}
		return ports.SSEEvent{Type: topic, Data: data}, audienceAll, nil
	default:
		return ports.SSEEvent{}, "", fmt.Errorf("unsupported topic %s", topic)
	}
}

func (b *Broker) labAudience(ctx context.Context, labID shared.LabInstanceID) (string, error) {
	if b.resolver == nil {
		return "", shared.ErrInvalidInput
	}
	ownerID, err := b.resolver.ResolveLabOwner(ctx, labID)
	if err != nil {
		return "", err
	}
	return "user:" + ownerID.String(), nil
}

func (b *Broker) add(c *client) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clients[c] = struct{}{}
	for _, audience := range c.audiences {
		if b.byAud[audience] == nil {
			b.byAud[audience] = map[*client]struct{}{}
		}
		b.byAud[audience][c] = struct{}{}
	}
}

func (b *Broker) drop(c *client) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.clients[c]; !ok {
		return
	}
	delete(b.clients, c)
	for _, audience := range c.audiences {
		delete(b.byAud[audience], c)
		if len(b.byAud[audience]) == 0 {
			delete(b.byAud, audience)
		}
	}
	close(c.done)
}

type client struct {
	events    chan ports.SSEEvent
	done      chan struct{}
	audiences []string
}

func writeEvent(w io.Writer, event ports.SSEEvent) error {
	if event.ID != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", event.ID); err != nil {
			return err
		}
	}
	if event.Type != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event.Type); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "data: %s\n\n", event.Data)
	return err
}

func labIDFrom(raw map[string]interface{}) (shared.LabInstanceID, error) {
	id := firstString(raw, "lab_id", "labId")
	if id == "" {
		return shared.LabInstanceID{}, shared.ErrInvalidInput
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return shared.LabInstanceID{}, err
	}
	return shared.LabInstanceID(parsed), nil
}

func firstString(raw map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := stringField(raw, key); value != "" {
			return value
		}
	}
	return ""
}

func stringField(raw map[string]interface{}, key string) string {
	value, _ := raw[key].(string)
	return value
}

func numberField(raw map[string]interface{}, keys ...string) float64 {
	for _, key := range keys {
		switch value := raw[key].(type) {
		case float64:
			return value
		case int:
			return float64(value)
		case json.Number:
			if parsed, err := value.Float64(); err == nil {
				return parsed
			}
		}
	}
	return 0
}
