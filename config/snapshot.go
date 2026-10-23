package config

import (
	"fmt"
	"reflect"
	"time"
)

// Snapshot is an immutable, pinned configuration revision. All methods are
// safe for concurrent use. Value and Unmarshal copy containers, so callers
// cannot modify the tree. The zero Snapshot represents an empty tree.
type Snapshot struct {
	data map[string]any
}

// Value returns a deep copy of the value at the dotted path. The empty path
// returns the whole tree. Numeric segments index into lists.
func (s *Snapshot) Value(path string) (any, bool) {
	v, ok := lookup(s.data, path)
	if !ok {
		return nil, false
	}
	return clone(v), true
}

// raw returns the value at path without copying it.
func (s *Snapshot) raw(path string) any {
	v, _ := lookup(s.data, path)
	return v
}

// String returns the value at path as a string, with the same conversions
// and defaults as Config.String.
func (s *Snapshot) String(path string) string {
	v, _ := toString(s.raw(path))
	return v
}

// Int returns the value at path as an int, with the same conversions and
// defaults as Config.Int.
func (s *Snapshot) Int(path string) int {
	v, err := toInt64(s.raw(path))
	if err != nil || int64(int(v)) != v {
		return 0
	}
	return int(v)
}

// Bool returns the value at path as a bool, with the same conversions and
// defaults as Config.Bool.
func (s *Snapshot) Bool(path string) bool {
	v, _ := toBool(s.raw(path))
	return v
}

// Float64 returns the value at path as a float64, with the same conversions
// and defaults as Config.Float64.
func (s *Snapshot) Float64(path string) float64 {
	v, _ := toFloat64(s.raw(path))
	return v
}

// Duration returns the value at path as a time.Duration, with the same
// conversions and defaults as Config.Duration.
func (s *Snapshot) Duration(path string) time.Duration {
	v, _ := toDuration(s.raw(path))
	return v
}

// Unmarshal decodes the subtree at prefix into dst, with the same conversion,
// field matching, defaults and error behavior as Config.Unmarshal.
func (s *Snapshot) Unmarshal(prefix string, dst any) error {
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fail(prefix, fmt.Errorf("%w, got %T", errNotPointer, dst))
	}
	// Treat a zero Snapshot like a loaded empty configuration, including when
	// decoding the root into an interface or validating its destination type.
	var root any = s.data
	if s.data == nil {
		root = map[string]any{}
	}
	src, ok := lookup(root, prefix)
	if !ok {
		return nil
	}
	return decode(prefix, src, rv.Elem())
}
