package errorx_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/nostosx/servex/errorx"
)

func TestWithRemote(t *testing.T) {
	local := errorx.New(errorx.Unavailable, "DOWN", "m")
	remote := local.WithRemote()
	if remote == local {
		t.Fatal("WithRemote returned the receiver")
	}
	if local.Remote() {
		t.Error("WithRemote modified the original")
	}
	if !remote.Remote() {
		t.Error("Remote() = false after WithRemote")
	}
	if !remote.Retryable() {
		t.Error("WithRemote changed Retryable; the immediate caller may still retry")
	}
	if remote.PropagatedRetryable() {
		t.Error("a remote retryable error must not propagate as retryable")
	}
	if !local.PropagatedRetryable() {
		t.Error("a local retryable error must propagate as retryable")
	}
	if !errors.Is(remote, local) || !errors.Is(local, remote) {
		t.Error("Is must ignore the remote mark")
	}
	if errorx.From(remote) != remote || !errorx.IsRetryable(remote) {
		t.Error("From or IsRetryable treat remote errors differently")
	}
}

func TestPropagatedRetryable(t *testing.T) {
	base := errorx.New(errorx.Internal, "R", "m")
	tests := []struct {
		name      string
		err       *errorx.Error
		retryable bool
		propagate bool
	}{
		{"nil", nil, false, false},
		{"local default", base, false, false},
		{"local explicit", base.WithRetryable(true), true, true},
		{"remote keeps body flag", base.WithRetryable(true).WithRemote(), true, false},
		{"remote default retryable", errorx.New(errorx.Aborted, "", "").WithRemote(), true, false},
		{"local decision after remote", base.WithRemote().WithRetryable(true), true, true},
		{"local refusal after remote", base.WithRetryable(true).WithRemote().WithRetryable(false), false, false},
		{"remote again clears local decision", base.WithRemote().WithRetryable(true).WithRemote(), true, false},
		{"clone keeps remote", base.WithRetryable(true).WithRemote().WithCause(errors.New("c")).WithMetadata(map[string]string{"k": "v"}), true, false},
		{"clone keeps local decision", base.WithRemote().WithRetryable(true).WithMetadata(nil), true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Retryable(); got != tt.retryable {
				t.Errorf("Retryable() = %v, want %v", got, tt.retryable)
			}
			if got := tt.err.PropagatedRetryable(); got != tt.propagate {
				t.Errorf("PropagatedRetryable() = %v, want %v", got, tt.propagate)
			}
		})
	}
}

func TestWrappedRemoteKeepsMark(t *testing.T) {
	remote := errorx.New(errorx.Unavailable, "", "m").WithRemote()
	wrapped := fmt.Errorf("call user service: %w", remote)
	e := errorx.From(wrapped)
	if !e.Remote() || e.PropagatedRetryable() {
		t.Errorf("From(%%w-wrapped remote) = remote %v, propagated %v; want true, false", e.Remote(), e.PropagatedRetryable())
	}
	if (*errorx.Error)(nil).Remote() {
		t.Error("nil *Error reported Remote")
	}
}
