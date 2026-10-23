package tracing

import "context"

// SpanKind describes the role of a span in a trace.
type SpanKind int

// Span kinds.
const (
	// SpanKindInternal is an operation internal to an application.
	SpanKindInternal SpanKind = iota
	// SpanKindServer covers the server-side handling of a synchronous request.
	SpanKindServer
	// SpanKindClient covers the client-side of a synchronous request.
	SpanKindClient
	// SpanKindProducer covers the sending of an asynchronous message.
	SpanKindProducer
	// SpanKindConsumer covers the processing of an asynchronous message.
	SpanKindConsumer
)

// Tracer starts spans.
type Tracer interface {
	// Start starts a span named name as a child of the span in ctx, if any,
	// and returns a context carrying the new span together with the span.
	Start(ctx context.Context, name string, kind SpanKind) (context.Context, Span)
}

// Span is a single timed operation within a trace.
type Span interface {
	// End completes the span.
	End()
	// SetAttribute records an attribute on the span.
	SetAttribute(key string, value any)
	// RecordError records err on the span and marks its status as an error.
	RecordError(err error)
	// TraceID returns the trace ID, or "" if the span context is invalid.
	TraceID() string
	// SpanID returns the span ID, or "" if the span context is invalid.
	SpanID() string
}

// Carrier is a key/value store used by a Propagator to inject and extract
// trace context. metadata.MD satisfies it structurally.
type Carrier interface {
	// Get returns the first value for key, or "".
	Get(key string) string
	// Set replaces the values for key with value.
	Set(key, value string)
	// Keys returns all keys in the carrier.
	Keys() []string
}

// Propagator moves trace context across process boundaries.
type Propagator interface {
	// Inject writes the trace context of ctx into c.
	Inject(ctx context.Context, c Carrier)
	// Extract reads trace context from c and returns a copy of ctx carrying it.
	Extract(ctx context.Context, c Carrier) context.Context
}

// spanKey is the context key under which the current Span is stored.
type spanKey struct{}

// ContextWithSpan returns a copy of ctx carrying s as the current span.
// A nil s is stored as a no-op span.
func ContextWithSpan(ctx context.Context, s Span) context.Context {
	if s == nil {
		s = noopSpan{}
	}
	return context.WithValue(ctx, spanKey{}, s)
}

// SpanFromContext returns the current span in ctx. It never returns nil: if
// ctx carries no span, a no-op span is returned.
func SpanFromContext(ctx context.Context) Span {
	if ctx != nil {
		if s, ok := ctx.Value(spanKey{}).(Span); ok && s != nil {
			return s
		}
	}
	return noopSpan{}
}
