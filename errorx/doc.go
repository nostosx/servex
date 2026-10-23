// Package errorx defines protocol-agnostic structured errors: a canonical
// Code whose values mirror the gRPC status codes, a machine-readable Reason,
// a human-readable Message, optional metadata and a retryable flag.
//
// Transports map Code to their own status representation; errorx itself
// knows nothing about any protocol.
package errorx
