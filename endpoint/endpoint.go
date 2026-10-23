package endpoint

import (
	"context"
	"slices"
)

// Endpoint handles a single request and returns its response.
//
// The concrete types of req and the response are defined by whoever registers
// the endpoint; transports and middleware treat them as opaque values.
type Endpoint func(ctx context.Context, req any) (any, error)

// Middleware decorates an Endpoint with cross-cutting behavior.
type Middleware func(next Endpoint) Endpoint

// Chain composes middleware in the order they are written: m[0] becomes the
// outermost layer and the last element wraps the endpoint directly.
//
// Nil elements are skipped. With no (non-nil) arguments Chain returns the
// identity middleware, which returns next unchanged.
func Chain(m ...Middleware) Middleware {
	ms := slices.DeleteFunc(slices.Clone(m), func(mw Middleware) bool { return mw == nil })
	return func(next Endpoint) Endpoint {
		for _, mw := range slices.Backward(ms) {
			next = mw(next)
		}
		return next
	}
}
