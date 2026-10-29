// Package websocket defines message-oriented WebSocket contracts without an
// SDK dependency. Implementations belong to servex-contrib/transport/websocketx.
package websocket

import (
	"context"
	"net/http"
)

// MessageType is a WebSocket data message opcode.
type MessageType int

const (
	Text   MessageType = 1
	Binary MessageType = 2
)

// CloseCode is a WebSocket close status code.
type CloseCode int

const (
	NormalClosure   CloseCode = 1000
	GoingAway       CloseCode = 1001
	PolicyViolation CloseCode = 1008
	InternalError   CloseCode = 1011
)

// Conn owns an upgraded connection. One reader may run concurrently with
// writers and Ping. Read must remain active to process control frames and
// acknowledge Ping. Canceling an I/O operation may close the connection.
// Close performs a bounded close handshake; CloseNow immediately releases it.
// HTTP server shutdown does not own upgraded connections: the application must
// track and close them. Read payloads belong to the caller.
type Conn interface {
	Read(context.Context) (MessageType, []byte, error)
	Write(context.Context, MessageType, []byte) error
	Ping(context.Context) error
	Subprotocol() string
	Close(CloseCode, string) error
	CloseNow() error
}

// Acceptor upgrades an HTTP request. It writes the handshake failure response
// itself. Callers must not write another HTTP response after Accept fails.
// The caller owns a successful connection, including its shutdown.
type Acceptor interface {
	Accept(http.ResponseWriter, *http.Request) (Conn, error)
}

// Dialer opens a connection with application headers. The caller owns the
// connection; the dial context bounds only the handshake, not its lifetime.
type Dialer interface {
	Dial(context.Context, string, http.Header) (Conn, error)
}

// CloseError exposes a peer's close code through errors.As without depending
// on an SDK-specific error type. Other errors need not implement it.
type CloseError interface {
	error
	CloseStatus() CloseCode
}
