package transport

import (
	"context"
	"net/url"

	"github.com/nostosx/servex/app"
)

// Server is a protocol server taking part in the application lifecycle.
// URL is valid once Start has returned successfully; its scheme names the
// protocol, such as "http" or "grpc".
type Server interface {
	app.Hook
	// URL returns the address the server actually listens on.
	URL() *url.URL
}

// Invoker performs the protocol-specific last step of a client call against
// the node at addr, which is in host:port form.
type Invoker interface {
	Invoke(ctx context.Context, addr string, req any) (any, error)
}

// InvokerFunc adapts an ordinary function to the Invoker interface.
type InvokerFunc func(ctx context.Context, addr string, req any) (any, error)

// Invoke calls f(ctx, addr, req).
func (f InvokerFunc) Invoke(ctx context.Context, addr string, req any) (any, error) {
	return f(ctx, addr, req)
}
