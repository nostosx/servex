package tracing

import "context"

// Noop returns a Tracer whose spans record nothing. It is safe for
// concurrent use and avoids allocation when ctx carries no span.
func Noop() Tracer { return noopTracer{} }

// NoopPropagator returns a Propagator that neither injects nor extracts
// anything. It is safe for concurrent use.
func NoopPropagator() Propagator { return noopPropagator{} }

// noopTracer is a Tracer that starts no-op spans.
type noopTracer struct{}

// Start returns a no-op span. If ctx carries a span other than a no-op span,
// the returned context carries the no-op span instead so that
// SpanFromContext reflects the span just started; otherwise ctx is returned
// unchanged, since it already yields a no-op span.
func (noopTracer) Start(ctx context.Context, _ string, _ SpanKind) (context.Context, Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s, ok := ctx.Value(spanKey{}).(Span); ok {
		if _, isNoop := s.(noopSpan); !isNoop {
			ctx = ContextWithSpan(ctx, noopSpan{})
		}
	}
	return ctx, noopSpan{}
}

// noopSpan is a Span that records nothing and has an invalid span context.
type noopSpan struct{}

func (noopSpan) End()                     {}
func (noopSpan) SetAttribute(string, any) {}
func (noopSpan) RecordError(error)        {}
func (noopSpan) TraceID() string          { return "" }
func (noopSpan) SpanID() string           { return "" }

// noopPropagator is a Propagator that does nothing.
type noopPropagator struct{}

func (noopPropagator) Inject(context.Context, Carrier) {}

func (noopPropagator) Extract(ctx context.Context, _ Carrier) context.Context { return ctx }
