// Package broker defines the publish/subscribe contract of servex and a
// consumer Hook that plugs a subscription into the application lifecycle.
//
// Message handlers are plain endpoint.Endpoint values, so server-side
// middleware can be reused for message consumption. A handler acknowledges a
// message by returning nil and rejects it by returning an error; errors that
// errorx reports as non-retryable send the message straight to the dead
// letter destination of the implementation.
package broker
