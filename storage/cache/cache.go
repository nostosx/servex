package cache

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned by Cache.Get when key is absent or expired.
var ErrNotFound = errors.New("cache: not found")

// Cache stores byte values by key. Implementations must be safe for
// concurrent use.
type Cache interface {
	// Get returns the value stored under key, or ErrNotFound on a miss.
	Get(ctx context.Context, key string) ([]byte, error)
	// Set stores value under key. A ttl <= 0 means the entry never expires.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// Delete removes key. Deleting an absent key is not an error.
	Delete(ctx context.Context, key string) error
}
