package selector

import (
	"context"

	"github.com/nostosx/servex/errorx"
)

// Node is a candidate endpoint for a call.
type Node struct {
	// Addr is the node address in host:port form.
	Addr string
	// Weight is the relative selection weight; values <= 0 count as 1.
	Weight int
	// Metadata holds arbitrary node attributes.
	Metadata map[string]string
}

// Done reports the outcome of a call made to a selected node, for use by
// stateful strategies such as p2c or per-node circuit breaking. A successful
// Select must return a non-nil Done.
type Done func(ctx context.Context, err error)

// Selector picks one node from nodes for a single call.
type Selector interface {
	// Select returns the chosen node and a non-nil Done that must be called
	// with the outcome of the call. It returns ErrNoAvailable when no node
	// can be chosen.
	Select(ctx context.Context, nodes []Node) (Node, Done, error)
}

// ErrNoAvailable is returned by selectors when there is no node to choose
// from. Its code is errorx.Unavailable and its reason "NO_AVAILABLE_NODE".
var ErrNoAvailable = errorx.New(errorx.Unavailable, "NO_AVAILABLE_NODE", "selector: no available node")

// weight returns the effective weight of n.
func (n Node) weight() int {
	return max(n.Weight, 1)
}
