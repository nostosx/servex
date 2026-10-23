package endpoint_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/nostosx/servex/endpoint"
)

// recorder returns a middleware that appends "<name>:before" and
// "<name>:after" around the call to next.
func recorder(name string, log *[]string) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req any) (any, error) {
			*log = append(*log, name+":before")
			resp, err := next(ctx, req)
			*log = append(*log, name+":after")
			return resp, err
		}
	}
}

func TestChainOrder(t *testing.T) {
	tests := []struct {
		name string
		mws  func(log *[]string) []endpoint.Middleware
		want []string
	}{
		{
			name: "no middleware is identity",
			mws:  func(*[]string) []endpoint.Middleware { return nil },
			want: []string{"endpoint"},
		},
		{
			name: "single",
			mws: func(log *[]string) []endpoint.Middleware {
				return []endpoint.Middleware{recorder("a", log)}
			},
			want: []string{"a:before", "endpoint", "a:after"},
		},
		{
			name: "first is outermost",
			mws: func(log *[]string) []endpoint.Middleware {
				return []endpoint.Middleware{recorder("a", log), recorder("b", log), recorder("c", log)}
			},
			want: []string{"a:before", "b:before", "c:before", "endpoint", "c:after", "b:after", "a:after"},
		},
		{
			name: "nil entries are skipped",
			mws: func(log *[]string) []endpoint.Middleware {
				return []endpoint.Middleware{nil, recorder("a", log), nil, recorder("b", log), nil}
			},
			want: []string{"a:before", "b:before", "endpoint", "b:after", "a:after"},
		},
		{
			name: "only nil entries is identity",
			mws:  func(*[]string) []endpoint.Middleware { return []endpoint.Middleware{nil, nil} },
			want: []string{"endpoint"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var log []string
			ep := func(_ context.Context, req any) (any, error) {
				log = append(log, "endpoint")
				return req, nil
			}
			got, err := endpoint.Chain(tt.mws(&log)...)(ep)(t.Context(), "req")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != "req" {
				t.Fatalf("response = %v, want %q", got, "req")
			}
			if !slices.Equal(log, tt.want) {
				t.Fatalf("call order = %v, want %v", log, tt.want)
			}
		})
	}
}

func TestChainIdentityReturnsSameEndpoint(t *testing.T) {
	wantErr := errors.New("boom")
	ep := func(context.Context, any) (any, error) { return nil, wantErr }
	_, err := endpoint.Chain()(ep)(t.Context(), nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestChainDoesNotAliasInput(t *testing.T) {
	var log []string
	mws := []endpoint.Middleware{recorder("a", &log), recorder("b", &log)}
	chain := endpoint.Chain(mws...)
	// Mutating the caller's slice after Chain returns must not affect the chain.
	mws[0], mws[1] = recorder("x", &log), nil

	ep := func(context.Context, any) (any, error) { return nil, nil }
	if _, err := chain(ep)(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	want := []string{"a:before", "b:before", "b:after", "a:after"}
	if !slices.Equal(log, want) {
		t.Fatalf("call order = %v, want %v", log, want)
	}
}

func TestChainConcurrentUse(t *testing.T) {
	add := func(n int) endpoint.Middleware {
		return func(next endpoint.Endpoint) endpoint.Endpoint {
			return func(ctx context.Context, req any) (any, error) {
				return next(ctx, req.(int)+n)
			}
		}
	}
	ep := endpoint.Chain(add(1), add(10), add(100))(func(_ context.Context, req any) (any, error) {
		return req, nil
	})

	var wg sync.WaitGroup
	for i := range 64 {
		wg.Go(func() {
			got, err := ep(t.Context(), i)
			if err != nil || got != i+111 {
				t.Errorf("ep(%d) = %v, %v; want %d", i, got, err, i+111)
			}
		})
	}
	wg.Wait()
}

func BenchmarkChain(b *testing.B) {
	pass := func(next endpoint.Endpoint) endpoint.Endpoint { return next }
	ep := endpoint.Chain(pass, pass, pass, pass)(func(context.Context, any) (any, error) { return nil, nil })
	ctx := b.Context()
	for b.Loop() {
		_, _ = ep(ctx, nil)
	}
}
