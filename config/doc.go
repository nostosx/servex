// Package config loads layered configuration from one or more sources,
// deep-merges them into a single tree and exposes typed accessors, a weakly
// typed reflection-based decoder and change notification.
//
// Keys are matched case-insensitively when sources are merged, so a later
// source overrides an earlier key regardless of spelling; the spelling of the
// latest source is kept. Lookups by dotted path try an exact match first and
// fall back to a case-insensitive one. Numeric path segments index into lists.
package config
