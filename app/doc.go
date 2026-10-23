// Package app is a dependency-free lifecycle container: it starts a list of
// Hooks in registration order, stops them in reverse order, aggregates their
// readiness and reacts to their runtime failures.
//
// Start moves Init through Starting to Running. A failed start rolls back
// previously started hooks and ends in Failed. Stop moves Running through
// Stopping to Stopped, or Failed if a hook or runtime failure occurred.
// Stopping an unstarted App moves Init directly to Stopped. Stopped and Failed
// are terminal; an App cannot be reused.
package app
