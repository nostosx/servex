package errorx_test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/nostosx/servex/errorx"
)

func TestCodeStringAndParse(t *testing.T) {
	tests := []struct {
		code errorx.Code
		name string
		num  uint32
	}{
		{errorx.OK, "OK", 0},
		{errorx.Canceled, "Canceled", 1},
		{errorx.Unknown, "Unknown", 2},
		{errorx.InvalidArgument, "InvalidArgument", 3},
		{errorx.DeadlineExceeded, "DeadlineExceeded", 4},
		{errorx.NotFound, "NotFound", 5},
		{errorx.AlreadyExists, "AlreadyExists", 6},
		{errorx.PermissionDenied, "PermissionDenied", 7},
		{errorx.ResourceExhausted, "ResourceExhausted", 8},
		{errorx.FailedPrecondition, "FailedPrecondition", 9},
		{errorx.Aborted, "Aborted", 10},
		{errorx.OutOfRange, "OutOfRange", 11},
		{errorx.Unimplemented, "Unimplemented", 12},
		{errorx.Internal, "Internal", 13},
		{errorx.Unavailable, "Unavailable", 14},
		{errorx.DataLoss, "DataLoss", 15},
		{errorx.Unauthenticated, "Unauthenticated", 16},
		{errorx.Code(17), "Code(17)", 17},
		{errorx.Code(99), "Code(99)", 99},
		{errorx.Code(4294967295), "Code(4294967295)", 4294967295},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if uint32(tt.code) != tt.num {
				t.Fatalf("%s has value %d, want %d", tt.name, uint32(tt.code), tt.num)
			}
			if got := tt.code.String(); got != tt.name {
				t.Fatalf("String() = %q, want %q", got, tt.name)
			}
			got, ok := errorx.ParseCode(tt.name)
			if !ok || got != tt.code {
				t.Fatalf("ParseCode(%q) = %v, %v; want %v, true", tt.name, got, ok, tt.code)
			}
		})
	}
}

func TestParseCodeRejects(t *testing.T) {
	for _, s := range []string{
		"", "notfound", "NOT_FOUND", " NotFound", "NotFound ", "Code", "Code(", "Code()",
		"Code(5)", "Code(0)", "Code(017)", "Code(+17)", "Code(-1)", "Code(17", "code(17)",
		"Code(4294967296)", "Code(1x)",
	} {
		if c, ok := errorx.ParseCode(s); ok {
			t.Errorf("ParseCode(%q) = %v, true; want false", s, c)
		}
	}
}

func TestErrorFormat(t *testing.T) {
	tests := []struct {
		name string
		err  *errorx.Error
		want string
	}{
		{
			name: "plain",
			err:  errorx.New(errorx.NotFound, "USER_NOT_FOUND", "user 42 not found"),
			want: "code=NotFound reason=USER_NOT_FOUND message=user 42 not found",
		},
		{
			name: "formatted",
			err:  errorx.Newf(errorx.InvalidArgument, "BAD_ID", "id %d is %s", 7, "negative"),
			want: "code=InvalidArgument reason=BAD_ID message=id 7 is negative",
		},
		{
			name: "with cause",
			err:  errorx.New(errorx.Internal, "DB", "query failed").WithCause(errors.New("connection reset")),
			want: "code=Internal reason=DB message=query failed: connection reset",
		},
		{
			name: "unknown code and empty fields",
			err:  errorx.New(errorx.Code(42), "", ""),
			want: "code=Code(42) reason= message=",
		},
		{
			name: "nil",
			err:  nil,
			want: "<nil>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUnwrap(t *testing.T) {
	cause := errors.New("cause")
	e := errorx.New(errorx.Internal, "R", "m").WithCause(cause)
	if !errors.Is(e, cause) {
		t.Fatal("errors.Is did not find the cause")
	}
	if e.Unwrap() != cause {
		t.Fatal("Unwrap did not return the cause")
	}
	if errorx.New(errorx.Internal, "R", "m").Unwrap() != nil {
		t.Fatal("Unwrap without cause must be nil")
	}
	var nilErr *errorx.Error
	if nilErr.Unwrap() != nil {
		t.Fatal("Unwrap on nil must be nil")
	}
}

func TestIs(t *testing.T) {
	base := errorx.New(errorx.NotFound, "USER_NOT_FOUND", "a")
	tests := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		{"same code and reason, different message", base, errorx.New(errorx.NotFound, "USER_NOT_FOUND", "b"), true},
		{"different reason", base, errorx.New(errorx.NotFound, "ORDER_NOT_FOUND", "a"), false},
		{"different code", base, errorx.New(errorx.Internal, "USER_NOT_FOUND", "a"), false},
		{"metadata and retryable ignored", base.WithMetadata(map[string]string{"k": "v"}).WithRetryable(true), base, true},
		{"wrapped error", fmt.Errorf("handler: %w", base), errorx.New(errorx.NotFound, "USER_NOT_FOUND", ""), true},
		{"non errorx target", base, errors.New("code=NotFound reason=USER_NOT_FOUND message=a"), false},
		{"nil errorx target", base, (*errorx.Error)(nil), false},
		{"target found through cause", errorx.New(errorx.Internal, "WRAP", "").WithCause(base), base, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errors.Is(tt.err, tt.target); got != tt.want {
				t.Fatalf("errors.Is = %v, want %v", got, tt.want)
			}
		})
	}
	var nilErr *errorx.Error
	if nilErr.Is(base) {
		t.Fatal("nil *Error must not match")
	}
}

func TestWithMethodsReturnCopies(t *testing.T) {
	orig := errorx.New(errorx.NotFound, "R", "m").WithMetadata(map[string]string{"a": "1"})
	snapshot := maps.Clone(orig.Metadata)

	withCause := orig.WithCause(errors.New("c"))
	withMeta := orig.WithMetadata(map[string]string{"a": "2", "b": "3"})
	withRetry := orig.WithRetryable(true)

	for name, c := range map[string]*errorx.Error{"WithCause": withCause, "WithMetadata": withMeta, "WithRetryable": withRetry} {
		if c == orig {
			t.Errorf("%s returned the receiver", name)
		}
		if c.Code != orig.Code || c.Reason != orig.Reason || c.Message != orig.Message {
			t.Errorf("%s changed the identity fields: %+v", name, c)
		}
		// Mutating a copy's metadata must not leak into the original.
		c.Metadata["leak"] = "x"
	}

	if orig.Unwrap() != nil {
		t.Error("WithCause modified the original cause")
	}
	if orig.Retryable() {
		t.Error("WithRetryable modified the original")
	}
	if !maps.Equal(orig.Metadata, snapshot) {
		t.Errorf("original metadata changed: %v", orig.Metadata)
	}
	if want := map[string]string{"a": "2", "b": "3", "leak": "x"}; !maps.Equal(withMeta.Metadata, want) {
		t.Errorf("merged metadata = %v, want %v", withMeta.Metadata, want)
	}

	// With* methods preserve the state set by earlier calls.
	chained := errorx.New(errorx.Internal, "R", "m").
		WithRetryable(true).
		WithCause(context.Canceled).
		WithMetadata(map[string]string{"k": "v"})
	if !chained.Retryable() || !errors.Is(chained, context.Canceled) || chained.Metadata["k"] != "v" {
		t.Errorf("chained With* lost state: %v", chained)
	}
}

func TestWithMetadataEmpty(t *testing.T) {
	e := errorx.New(errorx.Internal, "R", "m")
	if got := e.WithMetadata(nil); got.Metadata != nil {
		t.Errorf("WithMetadata(nil) on empty metadata = %v, want nil", got.Metadata)
	}
	if got := e.WithMetadata(map[string]string{}); got == e {
		t.Error("WithMetadata must return a copy")
	}
}

func TestRetryableDefaults(t *testing.T) {
	retryable := map[errorx.Code]bool{
		errorx.Unavailable:       true,
		errorx.ResourceExhausted: true,
		errorx.Aborted:           true,
		errorx.DeadlineExceeded:  true,
	}
	for c := range errorx.Code(20) {
		e := errorx.New(c, "", "")
		if got := e.Retryable(); got != retryable[c] {
			t.Errorf("%v.Retryable() = %v, want %v", c, got, retryable[c])
		}
		if got := e.WithRetryable(!retryable[c]).Retryable(); got == retryable[c] {
			t.Errorf("%v explicit override to %v ignored", c, !retryable[c])
		}
		if got := e.WithRetryable(retryable[c]).Retryable(); got != retryable[c] {
			t.Errorf("%v explicit setting %v ignored", c, retryable[c])
		}
	}
	var nilErr *errorx.Error
	if nilErr.Retryable() {
		t.Error("nil *Error must not be retryable")
	}
}

func TestFrom(t *testing.T) {
	notFound := errorx.New(errorx.NotFound, "USER_NOT_FOUND", "missing")
	plain := errors.New("boom")

	t.Run("nil", func(t *testing.T) {
		if errorx.From(nil) != nil {
			t.Fatal("From(nil) must be nil")
		}
	})
	t.Run("errorx itself", func(t *testing.T) {
		if got := errorx.From(notFound); got != notFound {
			t.Fatalf("From returned %p, want %p", got, notFound)
		}
	})
	t.Run("wrapped errorx", func(t *testing.T) {
		wrapped := fmt.Errorf("layer2: %w", fmt.Errorf("layer1: %w", notFound))
		if got := errorx.From(wrapped); got != notFound {
			t.Fatalf("From returned %v, want the wrapped *Error", got)
		}
	})
	t.Run("errorx in joined errors", func(t *testing.T) {
		if got := errorx.From(errors.Join(plain, notFound)); got != notFound {
			t.Fatalf("From returned %v, want the joined *Error", got)
		}
	})
	t.Run("errorx wins over context error", func(t *testing.T) {
		e := errorx.New(errorx.Unavailable, "DOWN", "").WithCause(context.Canceled)
		if got := errorx.From(fmt.Errorf("x: %w", e)); got != e {
			t.Fatalf("From returned %v, want %v", got, e)
		}
	})

	conversions := []struct {
		name string
		err  error
		code errorx.Code
	}{
		{"canceled", context.Canceled, errorx.Canceled},
		{"wrapped canceled", fmt.Errorf("op: %w", context.Canceled), errorx.Canceled},
		{"deadline", context.DeadlineExceeded, errorx.DeadlineExceeded},
		{"wrapped deadline", fmt.Errorf("op: %w", context.DeadlineExceeded), errorx.DeadlineExceeded},
		{"plain", plain, errorx.Unknown},
		{"wrapped plain", fmt.Errorf("op: %w", plain), errorx.Unknown},
		{"typed nil errorx in chain", fmt.Errorf("op: %w", (*errorx.Error)(nil)), errorx.Unknown},
	}
	for _, tt := range conversions {
		t.Run(tt.name, func(t *testing.T) {
			got := errorx.From(tt.err)
			if got == nil {
				t.Fatal("From returned nil")
			}
			if got.Code != tt.code {
				t.Fatalf("Code = %v, want %v", got.Code, tt.code)
			}
			if got.Unwrap() != tt.err {
				t.Fatalf("cause = %v, want the original error", got.Unwrap())
			}
			if !errors.Is(got, tt.err) {
				t.Fatal("converted error must wrap the original")
			}
			if got.Message == "" {
				t.Fatal("converted error must carry a message")
			}
			if errorx.CodeOf(tt.err) != tt.code {
				t.Fatalf("CodeOf = %v, want %v", errorx.CodeOf(tt.err), tt.code)
			}
		})
	}
}

func TestFromContextErrorsFromRealContexts(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got := errorx.CodeOf(ctx.Err()); got != errorx.Canceled {
		t.Errorf("CodeOf(canceled ctx) = %v", got)
	}

	dctx, dcancel := context.WithDeadline(t.Context(), time.Time{})
	defer dcancel()
	if got := errorx.CodeOf(dctx.Err()); got != errorx.DeadlineExceeded {
		t.Errorf("CodeOf(expired ctx) = %v", got)
	}
}

func TestCodeOfAndIsRetryable(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		code      errorx.Code
		retryable bool
	}{
		{"nil", nil, errorx.OK, false},
		{"plain", errors.New("x"), errorx.Unknown, false},
		{"canceled", context.Canceled, errorx.Canceled, false},
		{"deadline", context.DeadlineExceeded, errorx.DeadlineExceeded, true},
		{"unavailable", errorx.New(errorx.Unavailable, "", ""), errorx.Unavailable, true},
		{"wrapped unavailable", fmt.Errorf("w: %w", errorx.New(errorx.Unavailable, "", "")), errorx.Unavailable, true},
		{"override off", errorx.New(errorx.Unavailable, "", "").WithRetryable(false), errorx.Unavailable, false},
		{"override on", errorx.New(errorx.InvalidArgument, "", "").WithRetryable(true), errorx.InvalidArgument, true},
		{"not found", errorx.New(errorx.NotFound, "", ""), errorx.NotFound, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errorx.CodeOf(tt.err); got != tt.code {
				t.Errorf("CodeOf = %v, want %v", got, tt.code)
			}
			if got := errorx.IsRetryable(tt.err); got != tt.retryable {
				t.Errorf("IsRetryable = %v, want %v", got, tt.retryable)
			}
		})
	}
}

func TestConcurrentWith(t *testing.T) {
	base := errorx.New(errorx.Internal, "R", "m").WithMetadata(map[string]string{"a": "1"})
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Go(func() {
			e := base.WithMetadata(map[string]string{"i": fmt.Sprint(i)}).WithRetryable(i%2 == 0)
			if e.Metadata["a"] != "1" || e.Retryable() != (i%2 == 0) {
				t.Errorf("unexpected derived error %v", e)
			}
			_ = base.Error()
			_ = errors.Is(e, base)
		})
	}
	wg.Wait()
	if len(base.Metadata) != 1 || base.Retryable() {
		t.Fatalf("base modified: %v", base)
	}
}
