package app

import (
	"errors"
	"strconv"
)

// State is the lifecycle state of an App.
type State int

// Lifecycle states. StateStopped and StateFailed are terminal.
const (
	StateInit State = iota
	StateStarting
	StateRunning
	StateStopping
	StateStopped
	StateFailed
)

var stateNames = [...]string{
	StateInit:     "init",
	StateStarting: "starting",
	StateRunning:  "running",
	StateStopping: "stopping",
	StateStopped:  "stopped",
	StateFailed:   "failed",
}

// String returns the lowercase name of the state, such as "running".
// Unknown values are rendered as "State(n)".
func (s State) String() string {
	if s >= 0 && int(s) < len(stateNames) {
		return stateNames[s]
	}
	return "State(" + strconv.Itoa(int(s)) + ")"
}

// ErrInvalidState is returned, wrapped with the current state, when an
// operation is not allowed in the App's current state.
var ErrInvalidState = errors.New("app: invalid state")
