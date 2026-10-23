package errorx

import (
	"strconv"
	"strings"
)

// Code is a canonical error code. Its values correspond one-to-one to the
// gRPC status codes, but errorx does not depend on any protocol.
type Code uint32

// Canonical error codes.
const (
	// OK means no error.
	OK Code = iota
	// Canceled means the operation was canceled, typically by the caller.
	Canceled
	// Unknown means an error with no more specific classification.
	Unknown
	// InvalidArgument means the caller supplied an invalid argument.
	InvalidArgument
	// DeadlineExceeded means the deadline expired before the operation completed.
	DeadlineExceeded
	// NotFound means a requested entity was not found.
	NotFound
	// AlreadyExists means the entity the caller tried to create already exists.
	AlreadyExists
	// PermissionDenied means the caller is not allowed to perform the operation.
	PermissionDenied
	// ResourceExhausted means a resource such as a quota or rate limit was exhausted.
	ResourceExhausted
	// FailedPrecondition means the system is not in a state required for the operation.
	FailedPrecondition
	// Aborted means the operation was aborted, typically due to a concurrency conflict.
	Aborted
	// OutOfRange means the operation was attempted past the valid range.
	OutOfRange
	// Unimplemented means the operation is not implemented or supported.
	Unimplemented
	// Internal means an internal invariant was broken.
	Internal
	// Unavailable means the service is currently unavailable; retrying may help.
	Unavailable
	// DataLoss means unrecoverable data loss or corruption.
	DataLoss
	// Unauthenticated means the request lacks valid authentication credentials.
	Unauthenticated
)

// codeNames maps every known Code to its name. It is read-only.
var codeNames = [...]string{
	OK:                 "OK",
	Canceled:           "Canceled",
	Unknown:            "Unknown",
	InvalidArgument:    "InvalidArgument",
	DeadlineExceeded:   "DeadlineExceeded",
	NotFound:           "NotFound",
	AlreadyExists:      "AlreadyExists",
	PermissionDenied:   "PermissionDenied",
	ResourceExhausted:  "ResourceExhausted",
	FailedPrecondition: "FailedPrecondition",
	Aborted:            "Aborted",
	OutOfRange:         "OutOfRange",
	Unimplemented:      "Unimplemented",
	Internal:           "Internal",
	Unavailable:        "Unavailable",
	DataLoss:           "DataLoss",
	Unauthenticated:    "Unauthenticated",
}

// String returns the name of c, such as "NotFound". Unknown values are
// formatted as "Code(99)".
func (c Code) String() string {
	if int(c) < len(codeNames) {
		return codeNames[c]
	}
	return "Code(" + strconv.FormatUint(uint64(c), 10) + ")"
}

// ParseCode is the inverse of Code.String. It accepts the exact name of a
// known code, or "Code(N)" where N is not a known code. It reports false for
// any other input.
func ParseCode(s string) (Code, bool) {
	for c, name := range codeNames {
		if s == name {
			return Code(c), true
		}
	}
	rest, ok := strings.CutPrefix(s, "Code(")
	if !ok {
		return 0, false
	}
	digits, ok := strings.CutSuffix(rest, ")")
	if !ok {
		return 0, false
	}
	// The canonical-form check rejects leading zeros so that parsing stays
	// the exact inverse of String.
	n, err := strconv.ParseUint(digits, 10, 32)
	if err != nil || n < uint64(len(codeNames)) || strconv.FormatUint(n, 10) != digits {
		return 0, false
	}
	return Code(n), true
}
