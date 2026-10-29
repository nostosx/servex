// Package selector defines how a client picks one node out of the currently
// known set. Selection algorithms live in servex-contrib/selector.
//
// Stateful strategies receive call outcomes through the Done callback.
package selector
