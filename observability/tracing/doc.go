// Package tracing defines a minimal, backend-neutral tracing API (Tracer,
// Span and Propagator) and helpers to carry an explicitly supplied Span in a
// context. Concrete backends, including no-op and OpenTelemetry implementations,
// live in servex-contrib.
package tracing
