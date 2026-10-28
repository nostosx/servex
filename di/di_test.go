package di_test

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/nostosx/servex/di"
)

type stub struct {
	register func(reflect.Type, di.Provider) error
	resolve  func(reflect.Type) (any, error)
}

func (s stub) Register(t reflect.Type, p di.Provider) error { return s.register(t, p) }
func (s stub) Resolve(t reflect.Type) (any, error)          { return s.resolve(t) }

func TestProvidePreservesInterfaceBinding(t *testing.T) {
	var registered reflect.Type
	var provider di.Provider
	c := stub{register: func(key reflect.Type, p di.Provider) error {
		registered, provider = key, p
		return nil
	}}
	called := false
	if err := di.Provide[io.Reader](c, func(di.Resolver) (io.Reader, error) {
		called = true
		return strings.NewReader("hello"), nil
	}); err != nil {
		t.Fatal(err)
	}
	if registered != reflect.TypeFor[io.Reader]() || called {
		t.Fatalf("registered = %v, eager = %v", registered, called)
	}
	value, err := provider(c)
	if _, ok := value.(io.Reader); !ok || err != nil || !called {
		t.Fatalf("provider = %T, %v", value, err)
	}
}

func TestInvalidArguments(t *testing.T) {
	if err := di.Provide[int](nil, func(di.Resolver) (int, error) { return 0, nil }); !errors.Is(err, di.ErrInvalidArgument) {
		t.Fatalf("nil container = %v", err)
	}
	if err := di.Provide[int](stub{}, nil); !errors.Is(err, di.ErrInvalidArgument) {
		t.Fatalf("nil provider = %v", err)
	}
	if _, err := di.Resolve[int](nil); !errors.Is(err, di.ErrInvalidArgument) {
		t.Fatalf("nil resolver = %v", err)
	}
}

func TestSupplyOwnsValueImmediately(t *testing.T) {
	var p di.Provider
	c := stub{
		register: func(_ reflect.Type, provider di.Provider) error { p = provider; return nil },
		resolve:  func(reflect.Type) (any, error) { return p(nil) },
	}
	if err := di.Supply[io.Reader](c, strings.NewReader("hello")); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("registration failed")
	c.register = func(reflect.Type, di.Provider) error { return failure }
	c.resolve = func(reflect.Type) (any, error) { t.Fatal("resolved after failed registration"); return nil, nil }
	if err := di.Supply(c, 1); !errors.Is(err, failure) {
		t.Fatalf("Supply lost registration error: %v", err)
	}
}

func TestResolveChecksImplementation(t *testing.T) {
	for _, value := range []any{nil, "wrong type"} {
		r := stub{resolve: func(reflect.Type) (any, error) { return value, nil }}
		if _, err := di.Resolve[int](r); !errors.Is(err, di.ErrInvalidArgument) {
			t.Fatalf("Resolve(%v) = %v", value, err)
		}
	}
	r := stub{resolve: func(reflect.Type) (any, error) { return nil, nil }}
	if value, err := di.Resolve[io.Reader](r); value != nil || err != nil {
		t.Fatalf("nil interface = %v, %v", value, err)
	}
	if value, err := di.Resolve[*int](r); value != nil || err != nil {
		t.Fatalf("nil pointer = %v, %v", value, err)
	}
	failure := errors.New("constructor failed")
	r.resolve = func(reflect.Type) (any, error) { return nil, failure }
	if _, err := di.Resolve[int](r); !errors.Is(err, failure) {
		t.Fatalf("Resolve lost cause: %v", err)
	}
	defer func() {
		cause, ok := recover().(error)
		if !ok || !errors.Is(cause, failure) {
			t.Fatalf("MustResolve panic = %v", cause)
		}
	}()
	di.MustResolve[int](r)
}
