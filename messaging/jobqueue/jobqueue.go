package jobqueue

import (
	"context"
	"errors"
	"time"

	"github.com/nostosx/servex/endpoint"
	"github.com/nostosx/servex/metadata"
)

// defaultMaxRetry is the number of retries applied by ApplyOptions when no
// WithMaxRetry option is given.
const defaultMaxRetry = 3

// Job is a unit of background work.
type Job struct {
	// ID identifies the job. Implementations generate one when it is empty.
	ID string
	// Type selects the handler that processes the job.
	Type string
	// Payload is the opaque job argument.
	Payload []byte
	// Header carries job metadata, such as propagated trace context.
	Header metadata.MD
	// Attempt is the current attempt number, starting at 1.
	Attempt int
}

// EnqueueOptions holds the per-job options of Enqueue.
type EnqueueOptions struct {
	// Delay postpones the first attempt.
	Delay time.Duration
	// MaxRetry is the number of retries after the first attempt for
	// retryable errors, so a job runs at most MaxRetry+1 times. The default
	// is 3.
	MaxRetry int
	// UniqueKey, when not empty, makes Enqueue return ErrDuplicate for
	// another job with the same key enqueued within UniqueTTL.
	UniqueKey string
	// UniqueTTL is how long UniqueKey stays reserved.
	UniqueTTL time.Duration
}

// EnqueueOption sets a field of EnqueueOptions.
type EnqueueOption func(*EnqueueOptions)

// WithDelay postpones the first attempt of the job by d. Values <= 0 mean no
// delay.
func WithDelay(d time.Duration) EnqueueOption {
	return func(o *EnqueueOptions) { o.Delay = d }
}

// WithMaxRetry sets how many times a job failing with a retryable error is
// retried. Negative values are treated as 0.
func WithMaxRetry(n int) EnqueueOption {
	return func(o *EnqueueOptions) { o.MaxRetry = n }
}

// WithUnique reserves key for ttl: enqueueing another job with the same key
// before the reservation ends returns ErrDuplicate.
func WithUnique(key string, ttl time.Duration) EnqueueOption {
	return func(o *EnqueueOptions) {
		o.UniqueKey = key
		o.UniqueTTL = ttl
	}
}

// ApplyOptions returns the EnqueueOptions described by opts, starting from
// the defaults (MaxRetry 3). Nil options are skipped, and
// negative Delay and MaxRetry values are normalized to 0. It is intended for
// Enqueuer implementations.
func ApplyOptions(opts ...EnqueueOption) EnqueueOptions {
	o := EnqueueOptions{MaxRetry: defaultMaxRetry}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	o.Delay = max(o.Delay, 0)
	o.MaxRetry = max(o.MaxRetry, 0)
	return o
}

// ErrDuplicate is returned by Enqueue when a job with the same unique key is
// still reserved.
var ErrDuplicate = errors.New("jobqueue: duplicate job")

// Enqueuer submits jobs.
type Enqueuer interface {
	// Enqueue submits job and returns its ID.
	Enqueue(ctx context.Context, job *Job, opts ...EnqueueOption) (id string, err error)
}

// Runner processes jobs.
type Runner interface {
	// Run processes jobs until ctx ends, in which case it returns nil, or until
	// an unrecoverable error occurs. Jobs are dispatched to handlers by
	// Job.Type with the *Job as request. A handler returning nil completes the
	// job; a retryable error retries it with backoff up to MaxRetry times; a
	// non-retryable error fails it immediately.
	Run(ctx context.Context, handlers map[string]endpoint.Endpoint) error
}

// ReadyRunner is an optional extension of Runner for runners whose setup
// takes time, such as connecting to a server. NewWorker uses it when
// available, so that the worker hook's Start returns only once jobs are
// being processed.
type ReadyRunner interface {
	Runner
	// RunReady behaves like Run and calls the non-nil ready, at most once,
	// as soon as it processes jobs. Returning without calling ready means the
	// runner failed to start.
	RunReady(ctx context.Context, handlers map[string]endpoint.Endpoint, ready func()) error
}
