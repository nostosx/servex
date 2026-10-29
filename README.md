# servex

English | [简体中文](README.zh-CN.md)

servex is a Go microservice toolkit built as a light core with pluggable
implementations. This repository is the core: protocol-independent contracts,
the minimal framework runtime, using the Go standard library. Implementations
that need third-party SDKs, such as gRPC, Redis and OpenTelemetry, live in
[servex-contrib](https://github.com/nostosx/servex-contrib).

The current release is v1.4.0. Minor releases may contain breaking API
changes; each one is listed with its migration in [CHANGELOG.md](CHANGELOG.md).
Patch releases do not change the API.

Runnable HTTP and gRPC programs live in the separate
[servex-examples](https://github.com/nostosx/servex-examples) project.

v1.4.0 moves concrete backends into contrib. See
[the migration guide](BACKEND-MIGRATION.md) for moved import paths, explicit
Codec/Selector wiring and absent-span behavior. Earlier versions retain
their release-time layout.

## Getting started

Requires Go 1.27.3. Add servex to your application module:

```sh
go get github.com/nostosx/servex@v1.4.0
```

servex v1.4.0 has no external module dependencies. Core tags have no directory prefix: `v1.4.0`.

## Design principles

The core module allows only the standard library and kitx, tests included.
CI rejects other external dependencies, including `golang.org/x/...`. Beyond that,
three rules decide where code belongs:

1. **Concrete implementations live in contrib.** Core keeps contracts, data
   types and a minimal framework runtime: App, health.Group, registration and
   thin Consumer/Worker lifecycle adapters. HTTP, SQL, memory
   backends, JSON, random selection, no-op telemetry, cache Loader and the shared
   client engine live in servex-contrib.
2. **Keep configuration and logging backend-independent.** `config` and
   `logger` define contracts only. Viper, slog and Zap implementations live
   in servex-contrib and are explicitly injected. SQL keeps `database/sql`.
3. **Explicit over implicit.** No global service locator, automatic middleware
   ordering, `init()` registration or global mutable state. Constructors receive
   dependencies explicitly. Optional DI contracts live in `di`; container
   implementations belong to contrib and stay at the composition root.

The optional `di` package is contract-only: the core does not implement a
second DI engine. The contract is available since v1.3.0.

Packages follow a layered import rule, checked by `internal/archtest`.
[CONTRIBUTING.md](CONTRIBUTING.md#dependency-boundaries) lists the exact allowed
imports and approved kitx packages:

| Layer | Packages | May import |
|---|---|---|
| Foundation | `app` `logger` `di` `cqrs` `endpoint` `errorx` `metadata` `health` `encoding` `observability/*` | package allowlist; app may import logger |
| Contracts | `config` `selector` `discovery` `storage/*` `messaging/*` | foundation |
| Transport contracts | `transport` | `app` |
| Framework runtime | App, health.Group, registration and thin Consumer/Worker adapters | explicit core contracts; no backend selection |

## Packages

| Directory | Purpose |
|---|---|
| `transport/sse` | Contract-only SSE event, sender, acceptor, receiver and client APIs; implementation: contrib `transport/ssex` (v1.0.0). |
| `transport/websocket` | Contract-only WebSocket messages, connections, handshakes and close status; implementation: contrib `transport/websocketx` (v1.0.0). |
| `transport/rpc` | Typed unary request/response and stream contracts; Connect/gRPC/gRPC-Web adapter: contrib `transport/connectx` (v1.0.0). |
| `app` | Lifecycle container. Starts `Hook`s in registration order, stops them in reverse, rolls back a failed start, aggregates readiness (`App.Check`) and stops on runtime failures. `SignalContext` handles SIGINT/SIGTERM. |
| `di` | Optional `Container`/`Resolver` contracts and typed `Provide`, `Supply`, `Resolve`, `MustResolve` helpers. Implementations live in contrib; available since v1.3.0. |
| `cqrs` | Typed application `Handler`, `Command`, `Query` and explicit middleware composition; available since v1.1.0. |
| `endpoint` | `Endpoint`, `Middleware` and `Chain`: the protocol-agnostic unit of work shared by servers, clients, consumers and workers. |
| `errorx` | Structured errors: a `Code` mirroring the gRPC codes, a machine-readable reason, metadata and a retryable flag. Knows no protocol. |
| `metadata` | Request metadata carried in the context: header-like `MD` and the call description `Info` (kind, protocol, operation, header). |
| `health` | `Checker`, `CheckerFunc` and `Group`, which runs named checks concurrently. |
| `encoding` | `Codec` contract; codecs live in contrib. |
| `config` | Configuration contracts: `Source`, `Watchable`, `Reader`, `Config`. No loader or decoder. |
| `logger` | Backend-independent `Logger` and `Level`; no concrete or global backend. |
| `observability/metrics` | Backend-neutral `Meter`, `Counter`, `Gauge`, `Histogram` contracts. |
| `observability/tracing` | Backend-neutral `Tracer`, `Span`, `Propagator` contracts and context helpers. |
| `discovery` | `Registrar`, `Discoverer`, `Watcher`, and `Registration`, a Hook that registers running servers. |
| `selector` | `Selector`, `Node`, `Done` contracts; algorithms live in contrib. |
| `transport` | `Server`, `Invoker` and `InvokerFunc` contracts; the shared client engine lives in contrib/transport/client. |
| `storage/cache` | Byte-oriented `Cache` contract and miss error; read-through Loader lives in contrib/cache/loader. |
| `messaging/broker` | `Message`, `Publisher`, `Subscriber` and `NewConsumer`, a Hook that runs a subscription. Handlers are endpoints. |
| `messaging/jobqueue` | `Enqueuer`, `Runner`, enqueue options (delay, retry, uniqueness) and `NewWorker`, a Hook that runs job handlers. |
| `internal/...` | Private dependency checks, discovery address selection and consumer lifecycle helpers. |

## Typed application handlers

`cqrs` is available since v1.1.0. Keep business request/result types in the
application's command/query packages. `Command[Request, Result]`
and `Query[Request, Result]` are interfaces embedding `Handler[Request, Result]`.
`CommandFunc` and `QueryFunc` adapt functions to these contracts.
Structs with a matching `Handle` method also implement the contract. A command
may return a business result; use `struct{}` when it has no result payload.

For example, inside an application constructor with a consumer-owned writer:

```go
return cqrs.ChainCommand("catalog.create_product", cqrs.CommandFunc[Create, Product](
    func(ctx context.Context, cmd Create) (Product, error) {
        return products.Create(ctx, cmd)
    },
), middleware...)
```

`middleware` has type `[]cqrs.CommandMiddleware[Create, Product]`. Each wrapper
receives a value copy of `cqrs.Metadata{Kind, Operation}` at assembly time.
Use `ChainQuery` for read-only handlers. Operation names must be stable constants
such as `catalog.get_product`, never request IDs or user input. Empty names panic
during assembly. Command/query middleware share a generic implementation; their
names express intent rather than enforce read/write effects. Assemble the chain once
and reuse it. The first middleware is outermost; nil entries are skipped. It can
reject work, derive a context or transform results/errors. Nil handlers are
programming errors. See the executable [example and tests](cqrs/cqrs_test.go)
for a complete middleware implementation and ordering checks.

Handlers run synchronously and retain the caller's context, result and error
unless middleware changes them. Shared handler/middleware state must be safe for
concurrent calls. Queries stay read-only through application ports and tests;
this package does not enforce effects. Keep transport middleware on `endpoint`
and business middleware on `cqrs`; no conversion through `any` is required in
the application. Transactions and command idempotency remain explicit. Do not
apply automatic retries to writes without an application idempotency guarantee.

## Example

To try the following service in a new directory, initialize an application module:

```sh
mkdir servex-demo
cd servex-demo
go mod init example.com/servex-demo
go get github.com/nostosx/servex@v1.4.0
mkdir server client
```

Save the following as `server/main.go`. It provides one route, readiness tied
to the application state and graceful shutdown on SIGINT/SIGTERM:

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/nostosx/servex/app"
	"github.com/nostosx/servex/errorx"
	"github.com/nostosx/servex/health"
	"github.com/nostosx/servex-contrib/transport/httpx"
)

type GetUser struct {
	ID int `json:"id"`
}

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func getUser(_ context.Context, req *GetUser) (*User, error) {
	if req.ID <= 0 {
		return nil, errorx.New(errorx.InvalidArgument, "INVALID_ID", "id must be positive")
	}
	return &User{ID: req.ID, Name: "alice"}, nil
}

func run() error {
	var a *app.App
	srv := httpx.NewServer(
		httpx.WithAddress("127.0.0.1:8000"),
		httpx.WithReadHeaderTimeout(5*time.Second),
		httpx.WithReadTimeout(10*time.Second),
		httpx.WithWriteTimeout(10*time.Second),
		httpx.WithIdleTimeout(time.Minute),
		httpx.WithMaxBodyBytes(64<<10),
		// GET /readyz answers 200 only while the App is running.
		httpx.WithHealth(health.CheckerFunc(func(ctx context.Context) error { return a.Check(ctx) })),
	)
	// {id} and query parameters bind to the JSON fields of GetUser.
	httpx.RouteFunc(srv, "GET /v1/users/{id}", getUser)

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

Resolve the imports after saving the file, then start the server:

```sh
go mod tidy
go run ./server
```

In another terminal, `curl http://127.0.0.1:8000/v1/users/7` returns `{"id":7,"name":"alice"}`;
`/v1/users/0` returns status 400 with the errorx body.
The listener is local; configure authentication, authorization and TLS before
exposing a service. In-memory reference implementations provide no persistence
or cross-process coordination.

Save this client as `client/main.go` in the same demo module:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nostosx/servex-contrib/transport/httpx"
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

With the server running, run `go run ./client` from the demo module root in
another terminal. It prints `alice`. Stop the server with Ctrl+C.

To make the service discoverable, add
`discovery.Registration(registrar, discovery.Instance{Name: "user"}, srv)`
after the servers in `app.WithHooks`: hooks start in order, so the instance is
registered after the port is open. `Instance.Weight` sets its relative
selection weight for clients. The registration hook implements the optional
`Drain(ctx) error` hook method: when the App stops, it first drains hooks in
reverse order, which deregisters the instance, then waits for
`app.WithStopDelay` while servers finish in-flight requests, and only then
stops the hooks. A failed drain is retried once by the hook's `Stop`.

Give each server, consumer and worker of an App its own hook name with
`httpx.WithName`, `broker.WithName` or `jobqueue.WithName`. Consumers default
to `consumer:<topic>`, or `consumer:<topic>:<group>` for a consumer group.
A consumer or worker whose implementation opts in through
`broker.ReadySubscriber` or `jobqueue.ReadyRunner`, as the in-memory ones do,
starts only once it is subscribed or processing; its hook also takes part in
readiness. `database.Hook(name, db, database.WithReadiness(false))` keeps the
start-time ping but leaves the database out of readiness, for services that
can still serve some requests while it is down.

Middleware is an `endpoint.Middleware` applied in the order written
(`httpx.WithMiddleware(...)`, `httpx.WithClientMiddleware(...)`,
`broker.WithMiddleware(...)`). The recommended orders, using the
servex-contrib middleware, are:

```
server:   recovery -> tracing -> logging -> metrics -> auth -> ratelimit -> validate -> handler
client:   tracing -> logging -> metrics -> retry -> circuitbreaker -> timeout -> [select node -> invoke]
consumer: recovery -> tracing -> logging -> metrics -> handler
```

HTTP endpoint middleware runs after the request has been routed and its body
bound. Checks that must reject a request before its body is read, such as
authentication and rate limiting, and `net/http` instrumentation that should
also observe 404s, body-limit 413s and binding failures belong in
`httpx.WithHTTPMiddleware(...)`. It wraps the whole server handler, including
`HandleHTTP` routes and health probes; its first entry is outermost.

The HTTP error body carries `retryable`, the retry flag for the caller. Errors
that a client decodes from another service are marked remote
(`errorx.WithRemote`): the client's own retry middleware may still retry them,
but a server returning such an error, even wrapped with `%w`, reports it as not
retryable (`PropagatedRetryable`). Only the hop next to the failure retries, so
a chain of retrying services does not multiply the attempts reaching the
failing one. A handler that sets `WithRetryable` on a remote error makes that
decision its own, and the flag is sent as set. Clients that receive a body
without the field fall back to the code's default. A client transport failure is a
retryable `Unavailable` only when no connection was obtained or the request is
replayable under `net/http` rules (GET, HEAD, OPTIONS, TRACE, or an
`Idempotency-Key`/`X-Idempotency-Key` header). Otherwise the request may have
executed, and the `Unavailable` error is explicitly not retryable.

## Typed endpoints

`endpoint.Typed` adapts a function with concrete request and response types to
an `endpoint.Endpoint`; a request of another type fails with
`*endpoint.TypeError`, which servers report as 500 `Unknown`, and a nil request
passes the zero value. `httpx.RouteFunc` registers such a function with its
request bound into `*Req`:

```go
httpx.RouteFunc(srv, "GET /v1/users/{id}", func(ctx context.Context, req *GetUser) (*User, error) {
	return users.Get(ctx, req.ID)
})
```

The same adapter bridges a `cqrs` handler to any transport, consumer or
worker without conversions through `any`:

```go
httpx.Route[CreateProduct](srv, "POST /v1/products", endpoint.Typed(createProduct.Handle))
```

where `createProduct` is a `cqrs.Command[*CreateProduct, Product]`.

## Shared client engine

Target resolution, discovery watching, node observation and invocation orchestration
live in [contrib/transport/client](https://github.com/nostosx/servex-contrib/tree/main/transport/client).
HTTP and gRPC adapters share that implementation. The core supplies Server and
Invoker contracts; it does not select or implement a client execution policy.

Metric recording methods take the context of the measured operation
(`Counter.Add(ctx, v)`, `Gauge.Set(ctx, v)`, `Histogram.Observe(ctx, v)`), so
meters can link observations to the active span.

## HTTP limits and custom responses

Server options configure the native `http.Server` used by `Start`:

```go
srv := httpx.NewServer(
	httpx.WithReadHeaderTimeout(5*time.Second),
	httpx.WithReadTimeout(30*time.Second),
	httpx.WithWriteTimeout(30*time.Second),
	httpx.WithIdleTimeout(60*time.Second),
	httpx.WithMaxHeaderBytes(1<<20),
	httpx.WithMaxBodyBytes(8<<20),
)
```

Timeouts default to zero and follow `net/http` semantics: zero header and
idle timeouts inherit `ReadTimeout`; negative values disable that inheritance.
A zero header limit uses the standard library default. When another
`http.Server` hosts `srv` as a handler, configure transport settings there.
Connection timeouts do not impose an endpoint execution deadline.

`WithMaxBodyBytes` limits the entire body of `Handle` and `Route` requests,
including multipart framing and routes without a request factory. The default
is `httpx.DefaultMaxBodyBytes` (4 MiB, the gRPC default message size); raise it
for larger uploads. It checks known lengths early and reads any remainder before
calling the endpoint, using bounded memory. Nonpositive values disable the limit.
Detected overflow returns JSON status 413 with `InvalidArgument` and
`BODY_TOO_LARGE`; malformed input can return 400 before overflow is detected.
`HandleHTTP` handlers manage their own body limits. Multipart files remain
available through endpoint execution and response encoding, then are cleaned up.

Use a route encoder for a custom success status, headers, cookies or body:

```go
httpx.RouteFunc(srv, "POST /v1/users", getUser,
	httpx.WithResponseEncoder(func(w http.ResponseWriter, r *http.Request, reply any) error {
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

## Configuration and logging contracts

The core defines `config.Source`, `config.Watchable`, `config.Reader`,
`config.Config` and `logger.Logger` only. Construction, file/env loading,
Viper decoding, validation, reload orchestration and snapshot publication live
in [config/viper](https://github.com/nostosx/servex-contrib/tree/main/config/viper).
Use [logger/slog](https://github.com/nostosx/servex-contrib/tree/main/logger/slog) or
[logger/zap](https://github.com/nostosx/servex-contrib/tree/main/logger/zap) to inject a backend:

```go
log := slogadapter.New(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
application := app.New(app.WithLogger(log))
server := httpx.NewServer(httpx.WithLogger(log))
```

Here `slogadapter` imports `github.com/nostosx/servex-contrib/logger/slog`.
Omitting an optional logger disables logging; the core never selects a global
backend. Existing `*slog.Logger` values must be wrapped explicitly.
These contracts are available in servex v1.4.0. Use the matching contrib
configuration and logging adapters from their v1.0.0 releases.

## Cache loading

Core provides the byte-oriented Cache contract. The read-through Loader,
including singleflight, owner cancellation, timeout, cache-error fallback and
write-back policy, lives in [contrib/cache/loader](https://github.com/nostosx/servex-contrib/tree/main/cache/loader).
Construct it with an explicit Codec and close it to cancel and join shared work.

## servex-contrib

Optional dependency injection uses `di.Container` at the application composition
root. `servex-contrib/di/do` implements it with samber/do/v2. Business constructors
remain ordinary Go functions, and registration can stay in one `bootstrap.go`.
Use the published core and contrib module versions independently.

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
task check
task lint
task vulncheck
```

`check` rejects formatting, modernizer and tidy changes, vets and builds both
normal and `integration` configurations, enforces the standard-library and kitx
dependency boundary, and runs shuffled race tests. `lint` and `vulncheck` inspect both
configurations. All targets disable workspace files with `GOWORK=off`.
`task fmt`, `task fix` and `task tidy` explicitly modify files.

The eight public commands are `check`, `test`, `build`, `lint`, `vulncheck`,
`fmt`, `fix` and `tidy`; `task` defaults to `check`. Use `task test RACE=true`
for race tests or `task test COVER=true` for race tests with `coverage.out`
and function coverage. Internal check steps are hidden from both task lists.

See [CONTRIBUTING.md](CONTRIBUTING.md) for tool installation and change
requirements and the exact import table,
[CHANGELOG.md](CHANGELOG.md) for unreleased changes and
[RELEASING.md](RELEASING.md) for release gates. Sensitive issues follow
[SECURITY.md](SECURITY.md). The core has no service-backed integration tests.
CI uses the same Task targets; local success does not prove remote CI or
published-module availability.

## License

Licensed under the [MIT License](LICENSE).
