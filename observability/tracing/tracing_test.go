package tracing_test

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sync"
	"testing"

	"github.com/nostosx/servex/observability/tracing"
)

// recordingSpan is a minimal Span used to observe context propagation.
type recordingSpan struct {
	mu    sync.Mutex
	ended bool
	attrs map[string]any
	err   error
}

func (s *recordingSpan) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ended = true
}

func (s *recordingSpan) SetAttribute(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attrs == nil {
		s.attrs = make(map[string]any)
	}
	s.attrs[key] = value
}

func (s *recordingSpan) RecordError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

func (*recordingSpan) TraceID() string { return "trace" }
func (*recordingSpan) SpanID() string  { return "span" }

// mapCarrier is a minimal Carrier.
type mapCarrier map[string]string

func (c mapCarrier) Get(key string) string { return c[key] }
func (c mapCarrier) Set(key, value string) { c[key] = value }
func (c mapCarrier) Keys() []string        { return slices.Sorted(maps.Keys(c)) }

func TestSpanKindValues(t *testing.T) {
	kinds := []tracing.SpanKind{
		tracing.SpanKindInternal,
		tracing.SpanKindServer,
		tracing.SpanKindClient,
		tracing.SpanKindProducer,
		tracing.SpanKindConsumer,
	}
	for i, k := range kinds {
		if int(k) != i {
			t.Errorf("kind %d has value %d", i, int(k))
		}
	}
}

func TestSpanFromContextNeverNil(t *testing.T) {
	var nilCtx context.Context
	tests := []struct {
		name string
		ctx  context.Context
	}{
		{"empty context", t.Context()},
		{"nil context", nilCtx},
		{"nil span stored", tracing.ContextWithSpan(t.Context(), nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tracing.SpanFromContext(tt.ctx)
			if s == nil {
				t.Fatal("SpanFromContext returned nil")
			}
			if s.TraceID() != "" || s.SpanID() != "" {
				t.Fatalf("noop span has IDs %q/%q", s.TraceID(), s.SpanID())
			}
			// All methods must be callable.
			s.SetAttribute("k", 1)
			s.RecordError(errors.New("x"))
			s.End()
		})
	}
}

func TestContextWithSpanRoundTrip(t *testing.T) {
	span := &recordingSpan{}
	ctx := tracing.ContextWithSpan(t.Context(), span)
	got := tracing.SpanFromContext(ctx)
	if got != tracing.Span(span) {
		t.Fatalf("SpanFromContext = %v, want the stored span", got)
	}
	got.SetAttribute("http.method", "GET")
	got.RecordError(errors.New("boom"))
	got.End()
	if !span.ended || span.attrs["http.method"] != "GET" || span.err == nil {
		t.Fatalf("calls did not reach the stored span: %+v", span)
	}

	inner := &recordingSpan{}
	innerCtx := tracing.ContextWithSpan(ctx, inner)
	if tracing.SpanFromContext(innerCtx) != tracing.Span(inner) {
		t.Fatal("inner span does not shadow the outer one")
	}
	if tracing.SpanFromContext(ctx) != tracing.Span(span) {
		t.Fatal("outer context changed")
	}
}

func TestNoopTracer(t *testing.T) {
	tr := tracing.Noop()
	if tr == nil {
		t.Fatal("Noop returned nil")
	}

	t.Run("without parent span", func(t *testing.T) {
		parent := t.Context()
		ctx, span := tr.Start(parent, "op", tracing.SpanKindServer)
		if ctx != parent {
			t.Fatal("Start must return ctx unchanged when it carries no span")
		}
		if span == nil || span.TraceID() != "" || span.SpanID() != "" {
			t.Fatalf("unexpected span %v", span)
		}
		span.SetAttribute("k", "v")
		span.RecordError(errors.New("x"))
		span.End()
	})

	t.Run("with recording parent span", func(t *testing.T) {
		parent := &recordingSpan{}
		ctx, span := tr.Start(tracing.ContextWithSpan(t.Context(), parent), "op", tracing.SpanKindClient)
		span.RecordError(errors.New("x"))
		tracing.SpanFromContext(ctx).SetAttribute("k", "v")
		if parent.err != nil || parent.attrs != nil {
			t.Fatal("calls on the started span leaked into the parent span")
		}
		if tracing.SpanFromContext(ctx) == tracing.Span(parent) {
			t.Fatal("context must carry the started span, not the parent")
		}
	})

	t.Run("with noop parent span", func(t *testing.T) {
		parent := tracing.ContextWithSpan(t.Context(), nil)
		ctx, _ := tr.Start(parent, "op", tracing.SpanKindInternal)
		if ctx != parent {
			t.Fatal("Start must not rewrap a context that already carries a noop span")
		}
	})

	t.Run("nil context", func(t *testing.T) {
		var nilCtx context.Context
		ctx, span := tr.Start(nilCtx, "op", tracing.SpanKindInternal)
		if ctx == nil || span == nil {
			t.Fatal("Start(nil) must return a usable context and span")
		}
	})
}

func TestNoopPropagator(t *testing.T) {
	p := tracing.NoopPropagator()
	if p == nil {
		t.Fatal("NoopPropagator returned nil")
	}
	c := mapCarrier{"existing": "v"}
	ctx := tracing.ContextWithSpan(t.Context(), &recordingSpan{})
	p.Inject(ctx, c)
	if len(c) != 1 || c.Get("existing") != "v" {
		t.Fatalf("Inject modified the carrier: %v", c)
	}
	if got := p.Extract(t.Context(), mapCarrier{"traceparent": "00-abc-def-01"}); got != t.Context() {
		t.Fatal("Extract must return ctx unchanged")
	}
}

func TestNoopDoesNotAllocate(t *testing.T) {
	tr := tracing.Noop()
	p := tracing.NoopPropagator()
	ctx := t.Context()
	c := mapCarrier{}
	err := errors.New("x")
	allocs := testing.AllocsPerRun(100, func() {
		sctx, span := tr.Start(ctx, "op", tracing.SpanKindServer)
		span.SetAttribute("k", 1)
		span.RecordError(err)
		span.End()
		_ = tracing.SpanFromContext(sctx)
		p.Inject(sctx, c)
		_ = p.Extract(sctx, c)
		_ = tracing.Noop()
		_ = tracing.NoopPropagator()
	})
	if allocs != 0 {
		t.Fatalf("noop tracing allocated %v times per run, want 0", allocs)
	}
}

func TestNoopConcurrentUse(t *testing.T) {
	tr := tracing.Noop()
	p := tracing.NoopPropagator()
	parent := tracing.ContextWithSpan(t.Context(), &recordingSpan{})
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Go(func() {
			ctx, span := tr.Start(parent, "op", tracing.SpanKind(i%5))
			defer span.End()
			span.SetAttribute("i", i)
			p.Inject(ctx, mapCarrier{})
			_ = tracing.SpanFromContext(p.Extract(ctx, mapCarrier{}))
		})
	}
	wg.Wait()
}

func BenchmarkNoopStart(b *testing.B) {
	tr := tracing.Noop()
	ctx := b.Context()
	for b.Loop() {
		_, span := tr.Start(ctx, "op", tracing.SpanKindServer)
		span.End()
	}
}
