package errorx

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
)

// Error is a structured error carrying a canonical Code, a machine-readable
// Reason and a human-readable Message.
//
// Values returned by the With* methods are copies; an *Error should be
// treated as immutable once it is shared.
type Error struct {
	// Code is the canonical error code.
	Code Code
	// Reason is a machine-readable business reason, such as "USER_NOT_FOUND".
	Reason string
	// Message is a human-readable description.
	Message string
	// Metadata holds additional key/value details.
	Metadata map[string]string

	cause     error
	retryable *bool
}

// New returns an Error with the given code, reason and message.
func New(code Code, reason, message string) *Error {
	return &Error{Code: code, Reason: reason, Message: message}
}

// Newf returns an Error whose message is formatted with fmt.Sprintf.
func Newf(code Code, reason, format string, args ...any) *Error {
	return New(code, reason, fmt.Sprintf(format, args...))
}

// Error formats e as "code=NotFound reason=USER_NOT_FOUND message=...",
// followed by ": <cause>" when e has a cause.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	var b strings.Builder
	b.WriteString("code=")
	b.WriteString(e.Code.String())
	b.WriteString(" reason=")
	b.WriteString(e.Reason)
	b.WriteString(" message=")
	b.WriteString(e.Message)
	if e.cause != nil {
		b.WriteString(": ")
		b.WriteString(e.cause.Error())
	}
	return b.String()
}

// Unwrap returns the cause of e, or nil.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Is reports whether target is an *Error with the same Code and Reason as e.
// Message, metadata, cause and the retryable flag are ignored.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok || e == nil || t == nil {
		return false
	}
	return e.Code == t.Code && e.Reason == t.Reason
}

// WithCause returns a copy of e whose cause is err.
func (e *Error) WithCause(err error) *Error {
	c := e.clone()
	c.cause = err
	return c
}

// WithMetadata returns a copy of e whose metadata is the metadata of e merged
// with kv; keys in kv take precedence.
func (e *Error) WithMetadata(kv map[string]string) *Error {
	c := e.clone()
	if len(kv) > 0 {
		if c.Metadata == nil {
			c.Metadata = make(map[string]string, len(kv))
		}
		maps.Copy(c.Metadata, kv)
	}
	return c
}

// WithRetryable returns a copy of e whose retryable flag is explicitly set,
// overriding the default derived from Code.
func (e *Error) WithRetryable(retryable bool) *Error {
	c := e.clone()
	c.retryable = new(retryable)
	return c
}

// Retryable reports whether the failed operation may be retried. An explicit
// WithRetryable setting wins; otherwise Unavailable, ResourceExhausted,
// Aborted and DeadlineExceeded are retryable and every other code is not.
func (e *Error) Retryable() bool {
	if e == nil {
		return false
	}
	if e.retryable != nil {
		return *e.retryable
	}
	switch e.Code {
	case Unavailable, ResourceExhausted, Aborted, DeadlineExceeded:
		return true
	default:
		return false
	}
}

// clone returns a copy of e with its own metadata map. Cloning a nil *Error
// yields a zero Error.
func (e *Error) clone() *Error {
	if e == nil {
		return &Error{}
	}
	c := *e
	c.Metadata = maps.Clone(e.Metadata)
	return &c
}

// From converts any error into an *Error:
//
//   - nil yields nil;
//   - if an *Error is already in the chain of err, that *Error is returned;
//   - context.Canceled yields Canceled and context.DeadlineExceeded yields
//     DeadlineExceeded;
//   - anything else yields Unknown.
//
// For converted errors the original err is kept as the cause and the Message
// is a generic description, so internal details are not exposed through it.
func From(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := errors.AsType[*Error](err); ok && e != nil {
		return e
	}
	var e *Error
	switch {
	case errors.Is(err, context.Canceled):
		e = New(Canceled, "", "operation canceled")
	case errors.Is(err, context.DeadlineExceeded):
		e = New(DeadlineExceeded, "", "deadline exceeded")
	default:
		e = New(Unknown, "", "unknown error")
	}
	e.cause = err
	return e
}

// CodeOf returns the Code of err as determined by From; nil yields OK.
func CodeOf(err error) Code {
	if err == nil {
		return OK
	}
	return From(err).Code
}

// IsRetryable reports whether err is retryable as determined by From;
// nil yields false.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	return From(err).Retryable()
}
