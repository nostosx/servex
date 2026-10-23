package metadata

import "context"

// Kind describes the direction of a call.
type Kind string

// Call directions.
const (
	// KindServer marks a call received by this process.
	KindServer Kind = "server"
	// KindClient marks a call issued by this process.
	KindClient Kind = "client"
)

// Info describes a single call. On the server side it is written by the
// transport or consumer; on the client side by transport.Client.
type Info struct {
	// Kind is the call direction.
	Kind Kind
	// Protocol names the protocol, such as "http", "grpc", "broker" or "jobqueue".
	Protocol string
	// Operation is the operation name, such as "GET /v1/users/{id}", a topic or a job type.
	Operation string
	// Header holds the inbound header on the server side and the outbound
	// header on the client side, where middleware may write to it.
	Header MD
}

// infoKey is the context key under which Info is stored.
type infoKey struct{}

// NewContext returns a copy of ctx that carries info. The Header map is shared,
// allowing middleware to write outbound metadata; concurrent access follows MD's
// synchronization rules.
func NewContext(ctx context.Context, info Info) context.Context {
	return context.WithValue(ctx, infoKey{}, info)
}

// FromContext returns the Info stored in ctx, if any. Its Header is not copied.
func FromContext(ctx context.Context) (Info, bool) {
	if ctx == nil {
		return Info{}, false
	}
	info, ok := ctx.Value(infoKey{}).(Info)
	return info, ok
}
