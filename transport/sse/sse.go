// Package sse defines Server-Sent Events contracts. Wire encoding, parsing and
// HTTP integration belong to servex-contrib/transport/ssex.
package sse

import (
	"context"
	"net/http"
	"time"
)

// Event is a UTF-8 SSE event. Empty Type means "message". ID is the current
// last-event ID; an empty ID resets it. Retry is an optional non-negative,
// whole-millisecond reconnection delay. Data may contain multiple lines.
type Event struct {
	ID    string
	Type  string
	Data  string
	Retry *time.Duration
}

// Sender writes and flushes events or heartbeat comments. Calls must be
// serialized and finish before the owning HTTP handler returns. A write error
// ends the stream; callers must not retry a partially written event.
type Sender interface {
	Send(Event) error
	Comment(string) error
}

// Acceptor starts an SSE response. Validation errors leave the response
// uncommitted; transport errors may occur after commitment. The request context
// owns the sender's lifetime.
type Acceptor interface {
	Accept(http.ResponseWriter, *http.Request) (Sender, error)
}

// Receiver reads events until io.EOF or an error. Receive calls must be
// serialized. Close interrupts a blocked Receive and releases the response.
type Receiver interface {
	Receive() (Event, error)
	Close() error
}

// Client opens one event stream. The caller owns the returned Receiver and
// must close it. Cancellation terminates the HTTP request. Reconnection and
// replay policy belong to the caller; implementations must not replay writes.
type Client interface {
	Open(context.Context, string, http.Header) (Receiver, error)
}
