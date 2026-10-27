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
	name       string
	middleware []endpoint.Middleware
}

// WithName sets the hook name of the consumer, which identifies it in app
// logs and errors. The default is "consumer:<topic>", or
// "consumer:<topic>:<group>" for a non-empty group.
func WithName(name string) ConsumerOption {
	return func(o *consumerOptions) { o.name = name }
}

// WithMiddleware appends middleware applied to every message, in the order
// given: the first middleware is the outermost one. The metadata.Info of the
// message is already present in the context when the middleware runs.
func WithMiddleware(m ...endpoint.Middleware) ConsumerOption {
	return func(o *consumerOptions) {
		o.middleware = append(o.middleware, m...)
	}
}

// NewConsumer returns an app.Hook that consumes topic as a member of group
// through sub. It is named "consumer:<topic>", or "consumer:<topic>:<group>"
// for a non-empty group, unless WithName is given.
//
// Start launches sub.Subscribe in a new goroutine and returns immediately.
// When sub implements ReadySubscriber, Start runs SubscribeReady instead and
// waits until the subscription is ready, SubscribeReady returns or Start's
// ctx ends. An early return fails Start with its error, or with an error
// reporting the unexpected end for nil; on ctx expiry Start cancels the
// subscription and waits for it. The hook also implements Check(ctx) error,
// which fails until Start has returned successfully and after the
// subscription has ended, so it takes part in app readiness. Stop cancels the subscription and waits, bounded by its ctx, for Subscribe
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
	name := o.name
	if name == "" {
		name = "consumer:" + topic
		if group != "" {
			name += ":" + group
		}
	}
	if rs, ok := sub.(ReadySubscriber); ok {
		return runhook.NewReady("broker", name, func(ctx context.Context, ready func()) error {
			return rs.SubscribeReady(ctx, topic, group, handler, ready)
		})
	}
	return runhook.New("broker", name, func(ctx context.Context) error {
		return sub.Subscribe(ctx, topic, group, handler)
	})
}
