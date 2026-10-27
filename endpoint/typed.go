package endpoint

import (
	"context"
	"fmt"
	"reflect"
)

// TypeError reports a request whose dynamic type does not match the request
// type of an endpoint built with Typed. It is a programming error in the
// wiring between a transport and its handler; errorx.From maps it to Unknown.
type TypeError struct {
	// Got is the dynamic type of the request received.
	Got reflect.Type
	// Want is the request type the endpoint accepts.
	Want reflect.Type
}

// Error reports both types.
func (e *TypeError) Error() string {
	return fmt.Sprintf("endpoint: request type %v, want %v", e.Got, e.Want)
}

// Typed adapts a function with concrete request and response types to an
// Endpoint. A nil request passes the zero Req; a request of any other type
// than Req fails with a *TypeError without calling fn. The response of fn is
// returned as the endpoint response. Typed bridges typed application handlers,
// such as the Handle method of a cqrs handler: endpoint.Typed(h.Handle).
func Typed[Req, Res any](fn func(context.Context, Req) (Res, error)) Endpoint {
	return func(ctx context.Context, req any) (any, error) {
		var r Req
		if req != nil {
			v, ok := req.(Req)
			if !ok {
				return nil, &TypeError{Got: reflect.TypeOf(req), Want: reflect.TypeFor[Req]()}
			}
			r = v
		}
		return fn(ctx, r)
	}
}
