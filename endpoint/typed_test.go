package endpoint_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/nostosx/servex/endpoint"
)

type greet struct{ Name string }

func TestTyped(t *testing.T) {
	ep := endpoint.Typed(func(_ context.Context, req *greet) (string, error) {
		if req == nil {
			return "nobody", nil
		}
		return "hello " + req.Name, nil
	})

	if resp, err := ep(t.Context(), &greet{Name: "ada"}); err != nil || resp != "hello ada" {
		t.Errorf("typed call = %v, %v", resp, err)
	}
	if resp, err := ep(t.Context(), nil); err != nil || resp != "nobody" {
		t.Errorf("nil request = %v, %v; want the zero Req", resp, err)
	}

	resp, err := ep(t.Context(), greet{Name: "value"})
	te, ok := errors.AsType[*endpoint.TypeError](err)
	if !ok || resp != nil {
		t.Fatalf("mismatch = %v, %v; want *TypeError", resp, err)
	}
	if te.Got != reflect.TypeFor[greet]() || te.Want != reflect.TypeFor[*greet]() {
		t.Errorf("TypeError = %+v", te)
	}
	if want := "endpoint: request type endpoint_test.greet, want *endpoint_test.greet"; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestTypedPassesErrorsAndZeroValues(t *testing.T) {
	boom := errors.New("boom")
	ep := endpoint.Typed(func(_ context.Context, n int) (int, error) {
		if n < 0 {
			return 0, boom
		}
		return n * 2, nil
	})
	if resp, err := ep(t.Context(), nil); err != nil || resp != 0 {
		t.Errorf("nil request = %v, %v", resp, err)
	}
	if _, err := ep(t.Context(), -1); !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
	if _, err := ep(t.Context(), int64(1)); err == nil {
		t.Error("int64 request accepted by an int endpoint")
	}
}
