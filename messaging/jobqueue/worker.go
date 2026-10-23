package jobqueue

import (
	"context"

	"github.com/nostosx/servex/app"
	"github.com/nostosx/servex/endpoint"
	"github.com/nostosx/servex/internal/runhook"
	"github.com/nostosx/servex/metadata"
)

// WorkerOption configures a worker created by NewWorker.
type WorkerOption func(*workerOptions)

type workerOptions struct {
	middleware []endpoint.Middleware
}

// WithMiddleware appends middleware applied to every job, in the order given:
// the first middleware is the outermost one. The metadata.Info of the job is
// already present in the context when the middleware runs.
func WithMiddleware(m ...endpoint.Middleware) WorkerOption {
	return func(o *workerOptions) {
		o.middleware = append(o.middleware, m...)
	}
}

// NewWorker returns an app.Hook named "worker" that processes jobs from r with
// handlers, keyed by Job.Type. The handlers map is copied.
//
// Start launches r.Run in a new goroutine and returns immediately. Stop
// cancels Run and waits, bounded by its ctx, for it to return. If Run returns
// before Stop, whether with an error or with nil, the failure is reported
// through the hook's Err() <-chan error, which the app package watches. Stop
// returns any failure not already received from Err; nil and context.Canceled
// during shutdown are ignored. A worker can be started only once. Its run
// context retains Start's values but is canceled by Stop, independently of
// Start's context.
//
// Every job is handled with a context carrying
// metadata.Info{Kind: KindServer, Protocol: "jobqueue", Operation: job.Type, Header: job.Header}.
func NewWorker(r Runner, handlers map[string]endpoint.Endpoint, opts ...WorkerOption) app.Hook {
	var o workerOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	chain := endpoint.Chain(o.middleware...)
	wrapped := make(map[string]endpoint.Endpoint, len(handlers))
	for typ, h := range handlers {
		if h == nil {
			continue
		}
		next := chain(h)
		wrapped[typ] = func(ctx context.Context, req any) (any, error) {
			info := metadata.Info{
				Kind:      metadata.KindServer,
				Protocol:  "jobqueue",
				Operation: typ,
			}
			if job, ok := req.(*Job); ok && job != nil {
				info.Operation = job.Type
				info.Header = job.Header
			}
			return next(metadata.NewContext(ctx, info), req)
		}
	}
	return runhook.New("jobqueue", "worker", func(ctx context.Context) error {
		return r.Run(ctx, wrapped)
	})
}
