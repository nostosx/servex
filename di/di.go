package di

import (
	"errors"
	"fmt"
	"reflect"
)

var (
	ErrNotFound        = errors.New("di: dependency not found")
	ErrDuplicate       = errors.New("di: dependency already registered")
	ErrCycle           = errors.New("di: circular dependency")
	ErrClosed          = errors.New("di: container closed")
	ErrInvalidArgument = errors.New("di: invalid argument")
)

// Resolver resolves dependencies by their exact registered type. Interface
// bindings must be registered as that interface, not inferred from concrete
// types. Resolve returns errors wrapping ErrNotFound, ErrCycle or ErrClosed
// where applicable, and preserves errors returned by providers.
type Resolver interface {
	Resolve(reflect.Type) (any, error)
}

// Provider creates one dependency, resolving its own dependencies through r.
// Do not retain r: it records the current resolution path and is valid only
// for the duration of the call. Constructors should not start background work.
// Return an error for expected failures; implementations may recover panics.
type Provider func(r Resolver) (any, error)

// Container registers lazy singleton providers and resolves their values.
// Successful values are cached. Failed providers may be retried. Register
// rejects nil arguments and duplicate types without replacing a registration.
// It must not invoke the provider. A provider's result must be assignable to
// its registered type; invalid results wrap ErrInvalidArgument.
//
// Registration belongs to application assembly. Implementations must support
// concurrent resolution after registration, detect dependency cycles and
// reject registration/resolution after shutdown. Shutdown and health APIs are
// separate implementation capabilities, not required for dependency lookup.
// Go interfaces cannot have generic methods; use the typed helpers below.
type Container interface {
	Resolver
	Register(reflect.Type, Provider) error
}

// Provide registers a lazy singleton of T. To bind an interface, use that
// interface as T. A nil provider is rejected before touching the container.
func Provide[T any](c Container, provider func(Resolver) (T, error)) error {
	if c == nil || provider == nil {
		return fmt.Errorf("%w: nil container or provider", ErrInvalidArgument)
	}
	return c.Register(reflect.TypeFor[T](), func(r Resolver) (any, error) {
		return provider(r)
	})
}

// Supply registers an existing value as T. Its type, including an explicitly
// selected interface type, is preserved. Resource ownership follows the
// implementation's lifecycle contract, including before the first Resolve.
func Supply[T any](c Container, value T) error {
	if err := Provide(c, func(Resolver) (T, error) { return value, nil }); err != nil {
		return err
	}
	// Resolve the value immediately so lifecycle-aware implementations own it
	// even if no other provider ever requests it.
	_, err := c.Resolve(reflect.TypeFor[T]())
	return err
}

// Resolve obtains T from r, checking the implementation's returned type.
// Dependency registration completeness is checked at runtime, not by generics.
func Resolve[T any](r Resolver) (T, error) {
	var zero T
	if r == nil {
		return zero, fmt.Errorf("%w: nil resolver", ErrInvalidArgument)
	}
	t := reflect.TypeFor[T]()
	value, err := r.Resolve(t)
	if err != nil {
		return zero, err
	}
	if value == nil {
		switch t.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
			return zero, nil
		}
	}
	if result, ok := value.(T); ok {
		return result, nil
	}
	return zero, fmt.Errorf("%w: resolved %T for %v", ErrInvalidArgument, value, t)
}

// MustResolve is Resolve with panic-on-error semantics. Use only inside a
// provider whose implementation recovers resolution panics; use Resolve at
// the application entry point to handle assembly errors normally.
func MustResolve[T any](r Resolver) T {
	value, err := Resolve[T](r)
	if err != nil {
		panic(err)
	}
	return value
}
