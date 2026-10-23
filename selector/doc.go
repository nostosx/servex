// Package selector defines how a client picks one node out of the currently
// known set, together with a weighted random reference implementation.
//
// Stateful strategies such as p2c or per-node circuit breaking live in
// servex-contrib; they receive call outcomes through the Done callback.
package selector
