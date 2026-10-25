package cqrs_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/nostosx/servex/cqrs"
)

type request struct{ value int }
type result struct{ value int }
type contextKey struct{}

func TestAdaptersPreserveCall(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("business failure")
	ctx := context.WithValue(t.Context(), contextKey{}, "tenant")
	fn := func(got context.Context, req request) (result, error) {
		if got != ctx || req.value != 7 {
			t.Fatalf("context or request changed: %v, %+v", got, req)
		}
		return result{value: 9}, wantErr
	}
	for name, handler := range map[string]cqrs.Handler[request, result]{
		"command": cqrs.CommandFunc[request, result](fn),
		"query":   cqrs.QueryFunc[request, result](fn),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := handler.Handle(ctx, request{value: 7})
			if got.value != 9 || err != wantErr {
				t.Fatalf("got %+v, %v; want 9 and original error", got, err)
			}
		})
	}
}

func TestChainOrderAndOwnership(t *testing.T) {
	t.Parallel()
	var events []string
	var assembled int
	middleware := func(name string) cqrs.Middleware[request, result] {
		return func(_ cqrs.Metadata, next cqrs.Handler[request, result]) cqrs.Handler[request, result] {
			assembled++
			return cqrs.CommandFunc[request, result](func(ctx context.Context, req request) (result, error) {
				events = append(events, name+" before")
				got, err := next.Handle(ctx, req)
				events = append(events, name+" after")
				return got, err
			})
		}
	}
	base := cqrs.CommandFunc[request, result](func(_ context.Context, req request) (result, error) {
		events = append(events, "handler")
		return result(req), nil
	})
	chain := []cqrs.Middleware[request, result]{middleware("first"), nil, middleware("second")}
	handler := cqrs.ChainCommand("test.execute", base, chain...)
	// Assembly owns its wrappers; later caller changes must not affect execution.
	chain[0] = middleware("replacement")
	for range 2 {
		events = nil
		got, err := handler.Handle(t.Context(), request{value: 7})
		want := []string{"first before", "second before", "handler", "second after", "first after"}
		if got.value != 7 || err != nil || !slices.Equal(events, want) || assembled != 2 {
			t.Fatalf("got %+v, %v, %v; wrappers assembled %d times", got, err, events, assembled)
		}
	}
}

type structHandler struct{}

func (*structHandler) Handle(_ context.Context, req request) (result, error) {
	return result(req), nil
}

func TestChainIdentity(t *testing.T) {
	t.Parallel()
	base := &structHandler{}
	if cqrs.ChainCommand("test.execute", base) != base || cqrs.ChainCommand("test.execute", base, nil) != base {
		t.Fatal("empty chain must preserve handler identity")
	}
}

func TestMiddlewareCanRejectWithoutWriting(t *testing.T) {
	t.Parallel()
	denied := errors.New("permission denied")
	writes := 0
	base := cqrs.CommandFunc[request, struct{}](func(context.Context, request) (struct{}, error) {
		writes++
		return struct{}{}, nil
	})
	gate := func(_ cqrs.Metadata, _ cqrs.Handler[request, struct{}]) cqrs.Handler[request, struct{}] {
		return cqrs.CommandFunc[request, struct{}](func(context.Context, request) (struct{}, error) {
			return struct{}{}, denied
		})
	}
	_, err := cqrs.ChainCommand("test.execute", base, gate).Handle(t.Context(), request{})
	if err != denied || writes != 0 {
		t.Fatalf("got %v and %d writes; want original rejection and no writes", err, writes)
	}
}

func TestMiddlewarePropagatesCancellation(t *testing.T) {
	t.Parallel()
	cause := errors.New("request abandoned")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	base := cqrs.QueryFunc[request, result](func(got context.Context, _ request) (result, error) {
		if context.Cause(got) != cause {
			t.Fatal("cancellation cause was lost")
		}
		return result{}, got.Err()
	})
	wrap := func(_ cqrs.Metadata, next cqrs.Handler[request, result]) cqrs.Handler[request, result] {
		return cqrs.QueryFunc[request, result](func(ctx context.Context, req request) (result, error) {
			return next.Handle(ctx, req)
		})
	}
	if _, err := cqrs.ChainQuery("test.query", base, wrap).Handle(ctx, request{}); err != context.Canceled {
		t.Fatalf("got %v; want cancellation", err)
	}
}

func TestSharedChainConcurrentCalls(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	base := cqrs.QueryFunc[request, result](func(_ context.Context, req request) (result, error) {
		return result(req), nil
	})
	count := func(_ cqrs.Metadata, next cqrs.Handler[request, result]) cqrs.Handler[request, result] {
		return cqrs.QueryFunc[request, result](func(ctx context.Context, req request) (result, error) {
			calls.Add(1)
			return next.Handle(ctx, req)
		})
	}
	handler := cqrs.ChainQuery("test.query", base, count)
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Go(func() {
			got, err := handler.Handle(t.Context(), request{value: i})
			if got.value != i || err != nil {
				t.Errorf("request %d: got %+v, %v", i, got, err)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 32 {
		t.Fatalf("got %d calls; want exactly 32", calls.Load())
	}
}

func TestChainRejectsNilHandler(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		run  func()
	}{
		{"nil input", func() { cqrs.ChainCommand[request, result]("test.execute", nil) }},
		{"nil middleware result", func() {
			cqrs.ChainCommand("test.execute", &structHandler{}, func(_ cqrs.Metadata, _ cqrs.Handler[request, result]) cqrs.Handler[request, result] { return nil })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected assembly panic")
				}
			}()
			tc.run()
		})
	}
}

func ExampleChainCommand() {
	type create struct{ name string }
	type created struct{ id string }
	command := cqrs.CommandFunc[create, created](func(_ context.Context, req create) (created, error) {
		return created{id: req.name}, nil
	})
	validate := func(_ cqrs.Metadata, next cqrs.Handler[create, created]) cqrs.Handler[create, created] {
		return cqrs.CommandFunc[create, created](func(ctx context.Context, req create) (created, error) {
			if req.name == "" {
				return created{}, errors.New("name is required")
			}
			return next.Handle(ctx, req)
		})
	}
	handler := cqrs.ChainCommand("test.execute", command, validate)
	value, err := handler.Handle(context.Background(), create{name: "book"})
	fmt.Println(value.id, err)
	_, err = handler.Handle(context.Background(), create{})
	fmt.Println(err)
	// Output:
	// book <nil>
	// name is required
}

func TestOperationMetadata(t *testing.T) {
	t.Parallel()
	for _, kind := range []cqrs.Kind{cqrs.KindCommand, cqrs.KindQuery} {
		t.Run(string(kind), func(t *testing.T) {
			var seen []cqrs.Metadata
			observe := func(meta cqrs.Metadata, next cqrs.Handler[request, result]) cqrs.Handler[request, result] {
				seen = append(seen, meta)
				meta.Operation = "changed"
				return next
			}
			base := &structHandler{}
			var h cqrs.Handler[request, result]
			if kind == cqrs.KindCommand {
				h = cqrs.ChainCommand("catalog.get", base, observe, observe)
			} else {
				h = cqrs.ChainQuery("catalog.get", base, observe, observe)
			}
			got, err := h.Handle(t.Context(), request{value: 7})
			want := cqrs.Metadata{Kind: kind, Operation: "catalog.get"}
			if got.value != 7 || err != nil || len(seen) != 2 || seen[0] != want || seen[1] != want {
				t.Fatalf("metadata=%v result=%v error=%v", seen, got, err)
			}
		})
	}
}

func TestEmptyOperationRejected(t *testing.T) {
	t.Parallel()
	for _, kind := range []cqrs.Kind{cqrs.KindCommand, cqrs.KindQuery} {
		t.Run(string(kind), func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected empty operation panic")
				}
			}()
			if kind == cqrs.KindCommand {
				cqrs.ChainCommand(" \t", &structHandler{})
			} else {
				cqrs.ChainQuery(" \t", &structHandler{})
			}
		})
	}
}
