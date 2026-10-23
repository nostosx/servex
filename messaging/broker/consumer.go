package broker

import (
	"context"

	"github.com/nostosx/servex/app"
	"github.com/nostosx/servex/endpoint"
	"github.com/nostosx/servex/internal/runhook"
	"github.com/nostosx/servex/metadata"
)

// ConsumerOption configures a consumer created by NewConsumer.
type ConsumerOption func(*consumerOptions)

type consumerOptions struct {
	middleware []endpoint.Middleware
}

// WithMiddleware appends middleware applied to every message, in the order
// given: the first middleware is the outermost one. The metadata.Info of the
// message is already present in the context when the middleware runs.
func WithMiddleware(m ...endpoint.Middleware) ConsumerOption {
	return func(o *consumerOptions) {
		o.middleware = append(o.middleware, m...)
	}
}

// NewConsumer returns an app.Hook named "consumer:<topic>" that consumes topic
// as a member of group through sub.
//
// Start launches sub.Subscribe in a new goroutine and returns immediately.
// Stop cancels the subscription and waits, bounded by its ctx, for Subscribe
// to return. If Subscribe returns before Stop, whether with an error or with
// nil, the failure is reported through the hook's Err() <-chan error, which the
// app package watches. Stop returns any failure not already received from Err;
// nil and context.Canceled during shutdown are ignored. A consumer can be
// started only once. Its subscription context retains Start's values but is
// canceled by Stop, independently of Start's context.
//
// Every message is handled with a context carrying
// metadata.Info{Kind: KindServer, Protocol: "broker", Operation: topic, Header: msg.Header}.
func NewConsumer(sub Subscriber, topic, group string, h endpoint.Endpoint, opts ...ConsumerOption) app.Hook {
	var o consumerOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	next := endpoint.Chain(o.middleware...)(h)
	handler := func(ctx context.Context, req any) (any, error) {
		info := metadata.Info{
			Kind:      metadata.KindServer,
			Protocol:  "broker",
			Operation: topic,
		}
		if msg, ok := req.(*Message); ok && msg != nil {
			info.Header = msg.Header
		}
		return next(metadata.NewContext(ctx, info), req)
	}
	return runhook.New("broker", "consumer:"+topic, func(ctx context.Context) error {
		return sub.Subscribe(ctx, topic, group, handler)
	})
}
