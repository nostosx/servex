// Package logger defines backend-independent structured logging contracts.
// Applications inject implementations from servex-contrib/logger.
package logger

import "context"

// Level is a severity. Values between the named levels are permitted; adapters
// map them to their backend without introducing panic, fatal or process exit.
type Level int

const (
	LevelDebug Level = -4
	LevelInfo  Level = 0
	LevelWarn  Level = 4
	LevelError Level = 8
)

// Logger records structured events and must be safe for concurrent use.
// Fields are alternating string keys and values. Callers supply well-formed
// pairs; encoding and output policies belong to the implementation. ctx carries
// request context but must not be retained. Nil optional loggers disable
// logging; the core does not select a global or default backend.
type Logger interface {
	Enabled(context.Context, Level) bool
	Log(context.Context, Level, string, ...any)
}
