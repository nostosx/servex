// Package config defines configuration contracts. Implementations belong in
// servex-contrib; the core does not load, decode or publish configuration.
package config

import (
	"context"
	"time"
)

// Source loads a configuration tree. Returned maps and slices must not be
// mutated until Load has been consumed by the configuration implementation.
type Source interface {
	Load(context.Context) (map[string]any, error)
}

// Watchable reports changes until cancellation. Notifications may coalesce;
// they request a reload rather than describing every intermediate revision.
type Watchable interface {
	Source
	Watch(context.Context, func()) error
}

// Reader exposes one configuration view. Value and Unmarshal must not expose
// mutable containers owned by the implementation. Conversion rules belong to
// the implementation; missing typed values return their zero values.
type Reader interface {
	Value(string) (any, bool)
	String(string) string
	Int(string) int
	Bool(string) bool
	Float64(string) float64
	Duration(string) time.Duration
	Unmarshal(string, any) error
}

// Config is a concurrently readable, reloadable configuration. Snapshot pins
// one accepted revision. Failed reloads retain the last accepted revision.
// Watch blocks until cancellation or a terminal source error; implementations
// own watcher options, source construction and validation registration.
type Config interface {
	Reader
	Snapshot() Reader
	Reload(context.Context) error
	Watch(context.Context, func(Config), ...WatchOption) error
}

// WatchOptions carries watch policy without loading or publishing configuration.
type WatchOptions struct{ OnReloadError func(error) }

// WatchOption configures a watcher implementation.
type WatchOption func(*WatchOptions)
