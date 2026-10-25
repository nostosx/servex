// Package cqrs provides typed application handlers and explicit middleware
// composition. Business request and result types belong to the application.
//
// Commands may change state and return a result; queries must be read-only.
// These names express intent, not an effect system: the application enforces
// read/write boundaries through its ports and tests. A command without a result
// uses struct{} as its result type.
//
// Handlers execute synchronously. There is no registry, dispatch bus, automatic
// retry or transaction management. Contexts, results and errors pass through
// unchanged unless middleware deliberately transforms them. Implementations
// must honor cancellation and synchronize any shared mutable state.
package cqrs

import (
	"context"
	"slices"
	"strings"
)

// Handler handles one application request. Both Command and Query implement it,
// as can a struct with a matching Handle method.
type Handler[Request, Result any] interface {
	Handle(context.Context, Request) (Result, error)
}

// Command handles a state-changing request. A struct or CommandFunc can implement it.
type Command[Request, Result any] interface {
	Handler[Request, Result]
}

// Query handles a read-only request. A struct or QueryFunc can implement it.
type Query[Request, Result any] interface {
	Handler[Request, Result]
}

// CommandFunc adapts a function to Command. A nil function must not be invoked.
type CommandFunc[Request, Result any] func(context.Context, Request) (Result, error)

// Handle calls the command with the supplied context and request.
func (c CommandFunc[Request, Result]) Handle(ctx context.Context, request Request) (Result, error) {
	return c(ctx, request)
}

// QueryFunc adapts a function to Query. A nil function must not be invoked.
type QueryFunc[Request, Result any] func(context.Context, Request) (Result, error)

// Handle calls the query with the supplied context and request.
func (q QueryFunc[Request, Result]) Handle(ctx context.Context, request Request) (Result, error) {
	return q(ctx, request)
}

// Kind identifies an application operation, independently of its transport.
type Kind string

// Application operation kinds.
const (
	KindCommand Kind = "command"
	KindQuery   Kind = "query"
)

// Metadata describes a handler at assembly time. Operation is a stable name such
// as "catalog.create_product", never a request ID, resource ID or user input.
// Middleware receives a value copy; changes cannot affect other wrappers.
type Metadata struct {
	Kind      Kind
	Operation string
}

// Middleware decorates a typed handler using its assembly metadata. It may reject
// a request without calling next. It must not repeat writes unless the application
// explicitly guarantees idempotency.
type Middleware[Request, Result any] func(Metadata, Handler[Request, Result]) Handler[Request, Result]

// CommandMiddleware decorates a command. The alias shares implementation with
// query middleware; Metadata.Kind records the assembled chain's intent.
type CommandMiddleware[Request, Result any] = Middleware[Request, Result]

// QueryMiddleware decorates a query. It must preserve read-only behavior.
type QueryMiddleware[Request, Result any] = Middleware[Request, Result]

// ChainCommand wraps a command once using a stable operation name.
// The first middleware is outermost; nil entries are skipped. Assemble once and
// reuse the result. Concurrent calls require safe handlers and wrappers.
// Empty operation names and nil interface handlers panic during assembly.
// Typed nil handlers are programming errors and are not detected by reflection.
func ChainCommand[Request, Result any](operation string, handler Command[Request, Result], middleware ...CommandMiddleware[Request, Result]) Command[Request, Result] {
	return chain(Metadata{Kind: KindCommand, Operation: operation}, handler, middleware...)
}

// ChainQuery wraps a query once using a stable operation name.
// Ordering, ownership, concurrency and nil rules are the same as ChainCommand.
func ChainQuery[Request, Result any](operation string, handler Query[Request, Result], middleware ...QueryMiddleware[Request, Result]) Query[Request, Result] {
	return chain(Metadata{Kind: KindQuery, Operation: operation}, handler, middleware...)
}

func chain[Request, Result any](metadata Metadata, handler Handler[Request, Result], middleware ...Middleware[Request, Result]) Handler[Request, Result] {
	if strings.TrimSpace(metadata.Operation) == "" {
		panic("cqrs: empty operation")
	}
	if handler == nil {
		panic("cqrs: nil handler")
	}
	for _, m := range slices.Backward(middleware) {
		if m != nil {
			handler = m(metadata, handler)
			if handler == nil {
				panic("cqrs: middleware returned nil handler")
			}
		}
	}
	return handler
}
