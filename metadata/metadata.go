package metadata

import (
	"iter"
	"maps"
	"slices"
	"strings"
)

// MD is cross-protocol request metadata. Its constructors and accessor methods
// normalize keys to lower case. Map literals and direct assignments must use
// lower-case keys to work with those accessors.
//
// An MD is a plain map: concurrent reads are safe, but mutation must not run
// concurrently with any other access. Values returns the stored slice; callers
// must not modify it. Use Clone to obtain an independent copy.
// A nil MD can be read, but Set and Add require an initialized map.
type MD map[string][]string

// New builds an MD from a map of single values.
func New(kv map[string]string) MD {
	md := make(MD, len(kv))
	for k, v := range kv {
		md.Add(k, v)
	}
	return md
}

// Pairs builds an MD from alternating keys and values. Repeated keys
// accumulate values in order. It panics if called with an odd number of
// arguments.
func Pairs(kv ...string) MD {
	if len(kv)%2 == 1 {
		panic("metadata: Pairs got an odd number of arguments")
	}
	md := make(MD, len(kv)/2)
	for pair := range slices.Chunk(kv, 2) {
		md.Add(pair[0], pair[1])
	}
	return md
}

// Join merges mds into a new MD. Values of later MDs are appended after the
// values of earlier ones for the same key. Nil MDs are ignored.
func Join(mds ...MD) MD {
	out := make(MD)
	for _, md := range mds {
		for k, vs := range md {
			key := strings.ToLower(k)
			out[key] = append(out[key], vs...)
		}
	}
	return out
}

// Get returns the first value for key, or "" if there is none.
func (md MD) Get(key string) string {
	if vs := md[strings.ToLower(key)]; len(vs) > 0 {
		return vs[0]
	}
	return ""
}

// Values returns all values for key, or nil if there are none.
func (md MD) Values(key string) []string {
	return md[strings.ToLower(key)]
}

// Set replaces all values for key with value.
func (md MD) Set(key, value string) {
	md[strings.ToLower(key)] = []string{value}
}

// Add appends value to the values for key.
func (md MD) Add(key, value string) {
	key = strings.ToLower(key)
	md[key] = append(md[key], value)
}

// Delete removes key and all of its values.
func (md MD) Delete(key string) {
	delete(md, strings.ToLower(key))
}

// Keys returns the keys of md in ascending order.
func (md MD) Keys() []string {
	return slices.Sorted(maps.Keys(md))
}

// All returns an iterator over the keys of md in ascending order, yielding
// each key together with its values. The yielded slices must not be modified.
func (md MD) All() iter.Seq2[string, []string] {
	return func(yield func(string, []string) bool) {
		for _, k := range md.Keys() {
			if !yield(k, md[k]) {
				return
			}
		}
	}
}

// Clone returns a deep copy of md. Cloning a nil MD returns nil.
func (md MD) Clone() MD {
	if md == nil {
		return nil
	}
	out := make(MD, len(md))
	for k, vs := range md {
		out[k] = slices.Clone(vs)
	}
	return out
}
