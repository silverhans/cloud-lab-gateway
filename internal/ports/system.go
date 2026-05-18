package ports

import (
	"context"
	"io"
	"time"
)

// Clock abstracts time.Now() to make state-machine and timer logic deterministic
// in tests. Production uses adapter clock.System.
type Clock interface {
	Now() time.Time
}

// Random abstracts a CSPRNG. Used for SSH key generation, JWT secrets, etc.
// Tests can inject a deterministic source.
type Random interface {
	Read(p []byte) (n int, err error)
}

// SSEBroker fans out per-user SSE events to connected clients.
type SSEBroker interface {
	// Publish sends an event to all subscribers tagged with the given audience.
	// Audience is typically "user:{id}" or "course:{id}".
	Publish(audience string, event SSEEvent)

	// Subscribe registers a writer that receives events for the given audiences.
	// Blocks until ctx is cancelled.
	Subscribe(ctx context.Context, audiences []string, w io.Writer) error
}

// SSEEvent is a single SSE message.
type SSEEvent struct {
	Type string // e.g. "lab.state_changed"
	Data []byte // JSON
	ID   string // optional event ID
}
