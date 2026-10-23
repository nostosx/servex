# servex

English | [简体中文](README.zh-CN.md)

servex is a Go microservice toolkit built as a light core with pluggable
implementations. This repository is the core: protocol-independent contracts,
one reference implementation per contract, and shared primitives from the Go
standard library and [kitx](https://github.com/nostosx/kitx). Implementations
that need third-party SDKs, such as gRPC, Redis and OpenTelemetry, live in
[servex-contrib](https://github.com/nostosx/servex-contrib).

Status: preparing the first release, v1.0.0. No tagged release is available yet.
Current development
uses a local checkout through a `replace` directive (this is how
servex-contrib builds today). Follow the local setup below until the first
release has been published and its installation verified.

Runnable HTTP and gRPC programs live in the separate
[servex-examples](https://github.com/nostosx/servex-examples) project.

## Getting started

Requires Go 1.27.3. With a local `servex/` checkout next to your application,
add the core to the application's module. kitx v1.0.0 resolves from the Go
module proxy and needs no local checkout:

```sh
go mod edit -require=github.com/nostosx/servex@v0.0.0-00010101000000-000000000000
go mod edit -replace=github.com/nostosx/servex=../servex
go mod tidy
```

Adjust the path for your directory layout. After a core release is available,
use its version (for example `go get github.com/nostosx/servex@v1.0.0`)
instead. Core tags have no directory prefix: `v1.0.0`.

## Design principles

The core module allows only the standard library and kitx, tests included.
CI rejects other external dependencies, including `golang.org/x/...`. Beyond that,
three rules decide where code belongs:

1. **Core = contracts + one reference implementation per contract.** Trivial
   implementations live in the contract package (`encoding.JSON`,
   `config.Env`, `selector.Random`, the no-op meter and tracer); stateful ones
   live in a subdirectory (`*/memory`, `config/file`, `transport/httpx`).
   Every further implementation goes to servex-contrib.
2. **Do not wrap existing standard interfaces.** Logging is `*slog.Logger`,
   SQL is `database/sql`. servex only adds instrumentation, lifecycle and
   health checks around them; callers keep the native types.
3. **Explicit over implicit.** No global service locator, no DI container, no
   automatic middleware ordering, no `init()`, no global mutable state.
   Wiring is calling constructors; order is the order you write.

Packages follow a layered import rule, checked by `internal/archtest`.
[CONTRIBUTING.md](CONTRIBUTING.md#dependency-boundaries) lists the exact allowed
imports and approved kitx packages:

| Layer | Packages | May import |
|---|---|---|
| Foundation | `app` `endpoint` `errorx` `metadata` `health` `encoding` `observability/*` | standard library only; not each other |
| Contracts | `config` `selector` `discovery` `storage/*` `messaging/*` | foundation |
| Transport | `transport` | foundation, contracts, `kitx/backoff` |
| Implementations | reference subpackages such as `*/memory`, `config/file`, `transport/httpx` | their contract and the layers above, never another implementation |

## Packages

| Directory | Purpose |
|---|---|
| `app` | Lifecycle container. Starts `Hook`s in registration order, stops them in reverse, rolls back a failed start, aggregates readiness (`App.Check`) and stops on runtime failures. `SignalContext` handles SIGINT/SIGTERM. |
| `endpoint` | `Endpoint`, `Middleware` and `Chain`: the protocol-agnostic unit of work shared by servers, clients, consumers and workers. |
| `errorx` | Structured errors: a `Code` mirroring the gRPC codes, a machine-readable reason, metadata and a retryable flag. Knows no protocol. |
| `metadata` | Request metadata carried in the context: header-like `MD` and the call description `Info` (kind, protocol, operation, header). |
| `health` | `Checker`, `CheckerFunc` and `Group`, which runs named checks concurrently. |
| `encoding` | `Codec` interface and the JSON codec. |
| `config` | Layered configuration (`Source`, `Watchable`, `Config`) with typed accessors, `Unmarshal` and change notification; `Env` source. |
| `config/file` | File source; codec chosen by extension, changes detected by polling. |
| `observability/metrics` | Backend-neutral `Meter`, `Counter`, `Gauge`, `Histogram` and a no-op implementation. |
| `observability/tracing` | Backend-neutral `Tracer`, `Span`, `Propagator` and no-op implementations. |
| `discovery` | `Registrar`, `Discoverer`, `Watcher`, and `Registration`, a Hook that registers running servers. |
| `discovery/memory` | In-process registry for tests, examples and single-process deployments. |
| `selector` | `Selector`, `Node`, `Done` and the weighted `Random` selector. |
| `transport` | `Server`, `Invoker` and `Client`, which picks a node per call from a fixed target or from service discovery. |
| `transport/httpx` | `net/http` server and client: routing, request binding, codec negotiation, errorx-to-status mapping, `/livez` and `/readyz`. |
| `storage/cache` | Byte-oriented `Cache` contract and the generic read-through `Loader[T]` using kitx shared execution, an explicit owner, timeout and Close. |
| `storage/cache/memory` | Byte-copying adapter over kitx/ttlcache, with LRU and indexed per-entry TTL. |
| `storage/database` | `database/sql` integration: driver-level tracing and metrics, a lifecycle Hook with health check, and the `Tx` helper. Returns plain `*sql.DB`. |
| `messaging/broker` | `Message`, `Publisher`, `Subscriber` and `NewConsumer`, a Hook that runs a subscription. Handlers are endpoints. |
| `messaging/broker/memory` | In-process broker. |
| `messaging/jobqueue` | `Enqueuer`, `Runner`, enqueue options (delay, retry, uniqueness) and `NewWorker`, a Hook that runs job handlers. |
| `messaging/jobqueue/memory` | In-process job queue. |
| `internal/...` | Private dependency checks, discovery address selection and consumer lifecycle helpers. |

## Example

An HTTP service with one route, readiness tied to the application state and
graceful shutdown on SIGINT/SIGTERM:

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/nostosx/servex/app"
	"github.com/nostosx/servex/errorx"
	"github.com/nostosx/servex/health"
	"github.com/nostosx/servex/transport/httpx"
)

type GetUser struct {
	ID int `json:"id"`
}

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func getUser(_ context.Context, req any) (any, error) {
	r := req.(*GetUser)
	if r.ID <= 0 {
		return nil, errorx.New(errorx.InvalidArgument, "INVALID_ID", "id must be positive")
	}
	return &User{ID: r.ID, Name: "alice"}, nil
}

func run() error {
	var a *app.App
	srv := httpx.NewServer(
		httpx.Address("127.0.0.1:8000"),
		httpx.ReadHeaderTimeout(5*time.Second),
		httpx.ReadTimeout(10*time.Second),
		httpx.WriteTimeout(10*time.Second),
		httpx.IdleTimeout(time.Minute),
		httpx.MaxBodyBytes(64<<10),
		// GET /readyz answers 200 only while the App is running.
		httpx.Health(health.CheckerFunc(func(ctx context.Context) error { return a.Check(ctx) })),
	)
	// {id} and query parameters bind to the JSON fields of GetUser.
	httpx.Route[GetUser](srv, "GET /v1/users/{id}", getUser)

	a = app.New(app.WithHooks(srv))
	ctx, stop := app.SignalContext(context.Background())
	defer stop()
	return a.Run(ctx)
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
```

`curl http://127.0.0.1:8000/v1/users/7` returns `{"id":7,"name":"alice"}`;
`/v1/users/0` returns status 400 with the errorx body.
The listener is local; configure authentication, authorization and TLS before
exposing a service. In-memory reference implementations provide no persistence
or cross-process coordination.

Calling the route from another service:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nostosx/servex/transport/httpx"
)

type GetUser struct {
	ID int `json:"id"`
}

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// A "discovery:///user" target with httpx.WithDiscoverer resolves nodes
	// through service discovery instead.
	client, err := httpx.NewClient(ctx, "http://127.0.0.1:8000")
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	get := httpx.Call[User](client, "GET", "/v1/users/{id}")
	reply, err := get(ctx, &GetUser{ID: 7})
	if err != nil {
		log.Fatal(err) // an *errorx.Error for non-2xx replies
	}
	fmt.Println(reply.(*User).Name)
}
```

To make the service discoverable, add
`discovery.Registration(registrar, discovery.Instance{Name: "user"}, srv)`
after the servers in `app.WithHooks`: hooks start in order and stop in reverse,
so the instance is registered after the port is open and deregistered before
it closes.

Middleware is an `endpoint.Middleware` applied in the order written
(`httpx.Middleware(...)`, `httpx.WithClientMiddleware(...)`,
`broker.WithMiddleware(...)`). The recommended orders, using the
servex-contrib middleware, are:

```
server:   recovery -> tracing -> logging -> metrics -> auth -> ratelimit -> validate -> handler
client:   tracing -> logging -> metrics -> retry -> circuitbreaker -> timeout -> [select node -> invoke]
consumer: recovery -> tracing -> logging -> metrics -> handler
```

## HTTP limits and custom responses

Server options configure the native `http.Server` used by `Start`:

```go
srv := httpx.NewServer(
	httpx.ReadHeaderTimeout(5*time.Second),
	httpx.ReadTimeout(30*time.Second),
	httpx.WriteTimeout(30*time.Second),
	httpx.IdleTimeout(60*time.Second),
	httpx.MaxHeaderBytes(1<<20),
	httpx.MaxBodyBytes(8<<20),
)
```

Timeouts default to zero and follow `net/http` semantics: zero header and
idle timeouts inherit `ReadTimeout`; negative values disable that inheritance.
A zero header limit uses the standard library default. When another
`http.Server` hosts `srv` as a handler, configure transport settings there.
Connection timeouts do not impose an endpoint execution deadline.

A positive `MaxBodyBytes` limits the entire body of `Handle` and `Route`
requests, including multipart framing and routes without a request factory.
It checks known lengths early and reads any remainder before calling the
endpoint, using bounded memory. Nonpositive values disable the added limit.
Detected overflow returns JSON status 413 with `InvalidArgument` and
`BODY_TOO_LARGE`; malformed input can return 400 before overflow is detected.
`HandleHTTP` handlers manage their own body limits. Multipart files remain
available through endpoint execution and response encoding, then are cleaned up.

Use a route encoder for a custom success status, headers, cookies or body:

```go
httpx.Route[GetUser](srv, "POST /v1/users", getUser,
	httpx.ResponseEncoder(func(w http.ResponseWriter, r *http.Request, reply any) error {
		data, err := json.Marshal(reply)
		if err != nil {
			return err
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", "/v1/users/7")
		http.SetCookie(w, &http.Cookie{Name: "created", Value: "1", HttpOnly: true})
		w.WriteHeader(http.StatusCreated)
		_, err = w.Write(data)
		return err
	}),
)
```

The encoder runs after the middleware-wrapped endpoint succeeds, even for a
nil reply. The request carries transport metadata. Binding and endpoint errors
retain their fixed JSON response. Without an encoder, replies keep codec
negotiation and status 200, or 204 for nil. Return encoding errors before
writing whenever possible: before commitment they become JSON 500
`ENCODE_REPLY`; afterward they are logged without appending another response.
Use `http.ResponseController` for optional writer features such as flushing.
Encoding runs after endpoint middleware returns.

## Validated configuration updates

Keep `config.New(ctx, sources...)` for ordinary loading. To reject invalid
initial values and updates, construct with validators:

```go
cfg, err := config.NewWithOptions(ctx, []config.Source{config.Env("APP_")},
	config.WithValidator(func(ctx context.Context, candidate *config.Snapshot) error {
		var settings struct {
			Port int `json:"port"`
		}
		if err := candidate.Unmarshal("http", &settings); err != nil {
			return err
		}
		if settings.Port < 1 || settings.Port > 65535 {
			return errors.New("http.port must be between 1 and 65535")
		}
		return nil
	}),
)
if err != nil {
	return err
}
```

Validators run in registration order against the fully merged candidate,
including on construction. Only a successful load, validation and final
context check publish the new tree. Failed reloads retain the last accepted
values. Validators must decode into local destinations and must not reenter
`Reload` on the same config. Read a pinned version across several fields with
`cfg.Snapshot()`; `Value` and `Unmarshal` copy configuration containers.
Sources provide nested maps, lists and scalar values, rather than arbitrary
mutable Go object graphs.

`Watch` keeps its existing success callback. Use the additive API to observe
failed reload attempts as well:

```go
err = cfg.WatchWithOptions(ctx, func(current *config.Config) {
	apply(current.Snapshot())
}, config.WithReloadErrorHandler(func(err error) {
	report(err)
}))
```

Callbacks run synchronously outside the reload lock and never overlap within
one watch. Source notifications can coalesce. A rejected reload skips the
success callback and waits for the next notification or a manual `Reload`;
manual reloads report errors through their return value. Cancellation does
not generate a failure notification. Terminal source watch errors return
from `Watch`, while source-internal failures that do not attempt a reload are
outside the reload callback's scope. A load-only source such as `Env` needs a
manual reload or a notification from another source. Applying accepted values
to running components remains the application's responsibility.

## Cache Loader lifecycle

Construct a Loader with an owner context and positive shared execution timeout:

```go
loader, err := cache.NewLoader[User](owner, backend, 5*time.Second, cache.WithTTL(time.Minute))
if err != nil {
	return err
}
defer loader.Close()
```

Use a service or explicitly scoped owner that outlives individual requests.
Waiters pass their request contexts to Get and may cancel independently; load
callbacks inherit the owner's values and cancellation, never the initiating
request's context values.
Put tenant and permission scope in keys and pass shared credentials explicitly.
Close cancels and joins callbacks without closing the backend. The timeout,
owner cancellation and Close require callbacks and backends to honor context
cancellation. Distinct active keys still require application admission control.

Same-key callbacks must be interchangeable. Shared results containing pointers,
slices or maps must not be mutated. Calling Close or reentering the same key
from a load callback deadlocks and is prohibited. See
[Loader.Get](storage/cache/loader.go) for cache-error fallback and write-back
semantics.

## servex-contrib

[servex-contrib](https://github.com/nostosx/servex-contrib) holds the
implementations with third-party dependencies, one Go module per dependency
set (`github.com/nostosx/servex-contrib/<capability>/<impl>`), plus the
SDK-free `middleware` and `selector` modules. Contrib modules import
only the public packages of this module and never each other. Each
implementation is constructed with a plain function, so it can be wired by
hand or used directly as an fx or wire provider.

## Development

Use Go 1.27.3 and run from the repository root:

```sh
make check
make lint
make vulncheck
```

`check` rejects formatting, modernizer and tidy changes, vets and builds both
normal and `integration` configurations, enforces the standard-library and kitx
dependency boundary, and runs shuffled race tests. `lint` and `vulncheck` inspect both
configurations. All targets disable workspace files with `GOWORK=off`.
`make fmt`, `make fix` and `make tidy` explicitly modify files.

See [CONTRIBUTING.md](CONTRIBUTING.md) for tool installation and change
requirements and the exact import table,
[CHANGELOG.md](CHANGELOG.md) for unreleased changes and
[RELEASING.md](RELEASING.md) for release gates. Sensitive issues follow
[SECURITY.md](SECURITY.md). The core has no service-backed integration tests.
CI uses the same Make targets; local success does not prove remote CI or
published-module availability.

## License

Licensed under the [MIT License](LICENSE).
