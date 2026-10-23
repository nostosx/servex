package selector

import (
	"context"
	"math/rand/v2"
)

// Random returns a stateless Selector that picks nodes at random with
// probability proportional to their Weight. It uses math/rand/v2.
func Random() Selector {
	return &random{intN: rand.IntN}
}

// random is the weighted random Selector. intN is injectable for tests.
type random struct {
	intN func(n int) int
}

// noopDone is the Done returned by stateless selectors.
func noopDone(context.Context, error) {}

// Select implements Selector.
func (r *random) Select(_ context.Context, nodes []Node) (Node, Done, error) {
	switch len(nodes) {
	case 0:
		return Node{}, nil, ErrNoAvailable
	case 1:
		return nodes[0], noopDone, nil
	}
	total := 0
	for _, n := range nodes {
		total += n.weight()
	}
	pick := r.intN(total)
	for _, n := range nodes {
		pick -= n.weight()
		if pick < 0 {
			return n, noopDone, nil
		}
	}
	// Unreachable for a well-behaved intN; fall back to the last node.
	return nodes[len(nodes)-1], noopDone, nil
}
