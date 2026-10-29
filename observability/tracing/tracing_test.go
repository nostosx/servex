package tracing_test

import (
	"context"
	"errors"
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

func TestSpanFromContextAbsent(t *testing.T) {
	for _, tt := range []struct {
		name string
		ctx  context.Context
	}{
		{"empty context", t.Context()},
		{"nil context", nil},
		{"nil span stored", tracing.ContextWithSpan(t.Context(), nil)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tracing.SpanFromContext(tt.ctx); got != nil {
				t.Fatalf("absent span = %v, want nil", got)
			}
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

func TestNilSpanShadowsParent(t *testing.T) {
	parent := &recordingSpan{}
	outer := tracing.ContextWithSpan(t.Context(), parent)
	cleared := tracing.ContextWithSpan(outer, nil)
	if tracing.SpanFromContext(cleared) != nil {
		t.Fatal("nil span did not shadow parent")
	}
	if tracing.SpanFromContext(outer) != parent {
		t.Fatal("parent context changed")
	}
}
