// Package tracing defines a minimal, backend-neutral tracing API (Tracer,
// Span and Propagator) together with no-op implementations and helpers to
// carry the current Span in a context. Concrete backends such as
// OpenTelemetry live in servex-contrib.
package tracing
