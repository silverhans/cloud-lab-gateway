package asynq

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/cloud-lab-gateway/gateway/internal/ports"
)

// EventBus implements ports.EventBus on Redis pub/sub. It is used to fan out
// domain events (published by the outbox publisher) to in-process subscribers
// such as the SSE broker.
type EventBus struct {
	rdb   *redis.Client
	owned bool
}

var _ ports.EventBus = (*EventBus)(nil)

// NewEventBus connects to Redis at addr and owns the connection.
func NewEventBus(redisAddr string) *EventBus {
	return &EventBus{
		rdb:   redis.NewClient(&redis.Options{Addr: redisAddr}),
		owned: true,
	}
}

// NewEventBusWithClient reuses an existing Redis client. Close is a no-op for
// the shared client.
func NewEventBusWithClient(rdb *redis.Client) *EventBus {
	return &EventBus{rdb: rdb, owned: false}
}

// Close releases the Redis connection if this EventBus owns it.
func (b *EventBus) Close() error {
	if b.owned {
		return b.rdb.Close()
	}
	return nil
}

// Publish sends payload to all subscribers of topic.
func (b *EventBus) Publish(ctx context.Context, topic string, payload []byte) error {
	if err := b.rdb.Publish(ctx, topic, payload).Err(); err != nil {
		return fmt.Errorf("eventbus: publish %s: %w", topic, err)
	}
	return nil
}

// Subscribe returns a channel of messages for topic plus a cancel function.
// Calling cancel closes the subscription and drains the channel.
func (b *EventBus) Subscribe(topic string) (<-chan ports.Message, func(), error) {
	sub := b.rdb.Subscribe(context.Background(), topic)
	out := make(chan ports.Message, 32)
	go func() {
		defer close(out)
		for msg := range sub.Channel() {
			out <- ports.Message{Topic: msg.Channel, Payload: []byte(msg.Payload)}
		}
	}()
	cancel := func() { _ = sub.Close() }
	return out, cancel, nil
}
