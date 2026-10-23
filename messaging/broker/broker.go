package broker

import (
	"context"

	"github.com/nostosx/servex/endpoint"
	"github.com/nostosx/servex/metadata"
)

// Message is a unit of data exchanged through a broker.
type Message struct {
	// Topic is the destination the message is published to.
	Topic string
	// Key is an optional partitioning or ordering key.
	Key string
	// Header carries message metadata, such as propagated trace context.
	Header metadata.MD
	// Body is the opaque payload.
	Body []byte
}

// Publisher publishes messages.
type Publisher interface {
	// Publish sends msg to msg.Topic. It may block while the implementation
	// applies back pressure, but always honours ctx.
	Publish(ctx context.Context, msg *Message) error
}

// Subscriber consumes messages.
type Subscriber interface {
	// Subscribe consumes messages of topic until ctx ends, in which case it
	// returns nil, or until an unrecoverable error occurs.
	//
	// The req passed to h is a *Message and the response of h is ignored.
	// Returning nil acknowledges the message; returning an error rejects it.
	// Rejected messages whose error is not retryable (errorx.IsRetryable
	// reports false) go straight to the dead letter destination.
	//
	// Subscribers sharing a non-empty group split the messages of topic among
	// themselves; an empty group means every subscriber receives every
	// message (broadcast).
	Subscribe(ctx context.Context, topic, group string, h endpoint.Endpoint) error
}
