// Package rpc defines typed RPC application boundaries shared by HTTP RPC
// adapters. Protocol negotiation and generated SDK types stay in contrib.
package rpc

import (
	"context"
	"net/http"
)

// Request carries a message and transport headers. Headers are owned by the
// call and must not be retained or mutated concurrently.
type Request[T any] struct {
	Msg    *T
	Header http.Header
}

// Response carries a message, headers and trailing metadata. A successful
// unary handler must return a non-nil response and message.
type Response[T any] struct {
	Msg     *T
	Header  http.Header
	Trailer http.Header
}

// UnaryHandler handles one typed request. Both clients and application
// services may implement it. Calls may run concurrently.
type UnaryHandler[Req, Res any] interface {
	Call(context.Context, *Request[Req]) (*Response[Res], error)
}

// Receiver reads messages until io.EOF. Calls must be serialized and obey
// the context passed to the owning RPC handler.
type Receiver[T any] interface {
	Receive() (*T, error)
}

// Sender sends messages without automatic retries. Calls must be serialized.
// One Send and one Receive may run concurrently for a bidirectional stream.
type Sender[T any] interface {
	Send(*T) error
}

// ClientStream is the server view of a client-streaming RPC.
type ClientStream[T any] interface {
	Receiver[T]
	RequestHeader() http.Header
}

// ServerStream is the server view of a server-streaming RPC. Response headers
// must be set before the first Send, and trailers before the handler returns.
type ServerStream[T any] interface {
	Sender[T]
	ResponseHeader() http.Header
	ResponseTrailer() http.Header
}

// BidiStream is the server view of a bidirectional RPC. Streams and their
// metadata must not be used after the owning handler returns.
type BidiStream[Req, Res any] interface {
	ClientStream[Req]
	ServerStream[Res]
}
