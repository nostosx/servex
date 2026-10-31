# Changelog

User-visible changes are recorded here. Patch releases do not change the
public API. Starting with v1.2.0, minor releases may contain breaking API
changes; each one is listed under "Breaking changes" with its migration.

## Unreleased

## v1.5.0

Release date: 2026-10-31.

- Add backend-independent `storage/vector` and `storage/search` contracts for
  bounded, tenant-scoped vector/document adapters in servex-contrib. The core
  contains no SQL, embedding generation, index engine or new dependency.

## v1.4.0

Release date: 2026-10-29.

- Add contract-only `transport/sse`, `transport/websocket` and `transport/rpc`.
  SSE events/stream ownership, WebSocket handshakes/message connections and typed
  unary/streaming RPC boundaries contain no wire implementation or SDK imports.
  Implementations live in the independent contrib ssex, websocketx and connectx
  modules. Architecture tests enforce the contract-only boundary.

### Breaking changes

- Move HTTP, SQL, four memory backends, JSON, random selection and no-op
  telemetry implementations to servex-contrib. Old core implementation import
  paths and constructors are removed; see [migration guide](BACKEND-MIGRATION.md).
- Move shared transport.Client/NewClient to contrib/transport/client.Client/New
  and cache.Loader/NewLoader to contrib/cache/loader.Loader/New. ClientOption
  and LoaderOption become Option in their respective packages; their With*
  options and Loader errors move with them. Client still requires WithSelector;
  Loader requires WithCodec. HTTP/gRPC share the moved client engine.
- Define core as contracts plus the minimal framework runtime: App,
  health.Group, registration and thin Consumer/Worker lifecycle adapters stay.
  Core transport/cache contain no execution policy or kitx dependency; the
  unused core module requirement is removed. Architecture checks enforce this boundary.
- SpanFromContext returns nil when no Span is present; ContextWithSpan(nil)
  explicitly shadows parent spans without creating a no-op implementation.


- Configuration is contract-only: `config.Config` and `config.Reader` are
  interfaces. Constructors, Env, file sources, decoding, validators and
  snapshot publication moved to `servex-contrib/config/viper`.
  Replace `config.New/Env/WithValidator/WithReloadErrorHandler` with the
  corresponding Viper adapter APIs, `*config.Config` with `config.Config`,
  and `*config.Snapshot` with `config.Reader`.
- Lifecycle and HTTP logging accept `logger.Logger` instead of
  `*slog.Logger`. Wrap backends through `servex-contrib/logger/slog.New`
  or `logger/zap.New`. No injected logger means no logging; the core
  no longer selects `slog.Default()`.


## v1.3.0

Release date: 2026-10-28.

- Add optional `di.Resolver`, `di.Container` and typed `Provide`, `Supply`,
  `Resolve` and `MustResolve` helpers. The core defines contracts only;
  container implementations and lifecycle management remain in contrib.

- Development tooling: replace Make entry points with Task v3.53.1 in local
  checks, CI and contribution/release instructions. Use `task check`, `task lint`
  and `task vulncheck`; check coverage, build tags and failure gates are preserved.

## v1.2.0

Release date: 2026-10-27.

This release changes v1 APIs in place; the module path stays
`github.com/nostosx/servex`. Every breaking change is listed below with its
migration. It includes the unreleased v1.1.1 changes.

### Added

- `(*errorx.Error).WithRemote`, `Remote` and `PropagatedRetryable`: errors a
  client decodes from another service are marked remote. The receiving hop may
  still retry them, but a server returning them, even wrapped with `%w`,
  reports them as not retryable, so retries are not multiplied along a call
  chain. `WithRetryable` after `WithRemote` makes the flag a local decision
  that propagates as set.
- Optional hook method `Drain(ctx) error`. When a running App stops, it drains
  hooks one at a time in reverse order, then waits for the stop delay, then
  stops the hooks. Drain errors are joined with the stop errors and never abort
  the stop; a failed start's rollback does not drain.
- `discovery.Registration` deregisters in `Drain`, before the stop delay;
  `Stop` retries once if the drain failed.
- Opt-in start readiness: `broker.ReadySubscriber` (`SubscribeReady`) and
  `jobqueue.ReadyRunner` (`RunReady`). Consumers and workers over such
  implementations start only once subscribed or processing; an early end fails
  Start instead of being reported later, and Start's context bounds the wait.
  Consumer and worker hooks implement `Check`. Both in-memory implementations
  opt in.
- `database.WithReadiness(bool)` as a `database.Hook` option.
- `endpoint.Typed` and `*endpoint.TypeError`, `httpx.RouteFunc`.
- `discovery.Instance.Weight`, copied to `selector.Node.Weight` by
  `transport.Client`.
- `transport.WithNodeObserver`, called with a deep copy whenever the client's
  node set changes.
- Hook names: `httpx.WithName`, `broker.WithName` and `jobqueue.WithName`.

### Breaking changes

1. **Retry flag across hops.** HTTP error bodies now carry
   `PropagatedRetryable` instead of `Retryable`. A service that returns an
   error it received from another service, directly or wrapped, now tells its
   caller not to retry. Migration: nothing for typical services. A handler that
   wants its caller to retry a downstream failure sets it explicitly:
   `return nil, errorx.From(err).WithRetryable(true)`.
2. **Drain before stop delay.** `discovery.Registration` deregisters when the
   App starts stopping instead of after the stop delay. Hooks that already
   have a `Drain(ctx) error` method are now called by `app`. Migration: rename
   such a method if it is not meant as a drain step; keep releasing all
   resources in `Stop`, which still runs alone after a failed start.
3. **Consumer and worker start.** With a `ReadySubscriber` or `ReadyRunner`,
   Start blocks until ready and returns subscription failures that were
   previously reported through `Err` after the App was running. A second
   `memory.Queue` worker now fails Start. Consumer and worker hooks now take
   part in `App.Check`. Migration: bound Start with a context or
   `app.WithStopTimeout` as before; expect start errors from consumer setup.
4. **`database.Hook` signature.** `Hook(name, db)` becomes
   `Hook(name, db, opts ...HookOption)`. Calls compile unchanged; code storing
   the function value as `func(string, *sql.DB) app.Hook` must wrap it.
   Use `database.WithReadiness(false)` to leave the database out of readiness.
5. **Typed endpoints.** No existing API changes; replace `req.(*T)` assertions
   with `httpx.RouteFunc(srv, pattern, fn)` or `endpoint.Typed(fn)`. Bridge a
   cqrs handler with `endpoint.Typed(h.Handle)`.
6. **`discovery.Instance.Weight`.** A new field: unkeyed `Instance` literals
   no longer compile; use keyed fields. Nodes built from discovery now carry
   the instance weight instead of 0, which changes weighted selection when
   registries set it.
7. **Node observer.** New option only; no migration.
8. **Metrics take a context.** `Counter.Add(ctx, v)`, `Gauge.Set(ctx, v)`,
   `Gauge.Add(ctx, v)` and `Histogram.Observe(ctx, v)`. Migration: pass the
   context of the measured operation at every call site and add the parameter
   to every `metrics.Counter`, `Gauge` and `Histogram` implementation.
9. **`With*` option names.**
   - httpx server: `Address`, `ReadTimeout`, `ReadHeaderTimeout`,
     `WriteTimeout`, `IdleTimeout`, `MaxHeaderBytes`, `MaxBodyBytes`,
     `Middleware`, `HTTPMiddleware`, `Codecs`, `Health` and `Logger` become
     `WithAddress`, `WithReadTimeout`, `WithReadHeaderTimeout`,
     `WithWriteTimeout`, `WithIdleTimeout`, `WithMaxHeaderBytes`,
     `WithMaxBodyBytes`, `WithMiddleware`, `WithHTTPMiddleware`, `WithCodecs`,
     `WithHealth` and `WithLogger`.
   - httpx routes: `Operation` and `ResponseEncoder` become `WithOperation` and
     `WithResponseEncoder`.
   - jobqueue: `Delay`, `MaxRetry` and `Unique` become `WithDelay`,
     `WithMaxRetry` and `WithUnique`.
   - config: `New(ctx, sources...)` and `NewWithOptions(ctx, sources, opts...)`
     become `New(ctx, sources, opts...)`, with sources as a slice:
     `config.New(ctx, []config.Source{a, b})`. `WatchWithOptions` is removed;
     `Watch(ctx, fn, opts...)` takes the watch options.
10. **Hook names.** Consumers in a group are now named
    `consumer:<topic>:<group>` instead of `consumer:<topic>`, which changes
    app log fields and error messages. Migration: update log queries and alerts
    or pin the old name with `broker.WithName("consumer:<topic>")`. Give
    several HTTP servers or workers of one App distinct names with
    `httpx.WithName` and `jobqueue.WithName`.

### Adoption in servex-contrib and servex-examples

Contrib modules and examples keep requiring v1.1.0 until v1.2.0 is published.
Adopting it requires:

- `transport/grpcx`: mark decoded errors with `WithRemote` and encode the
  retry decision from `PropagatedRetryable`; consider a `WithName` server
  option. Replace `req.(*T)` in tests and docs with `endpoint.Typed`.
- `middleware/metrics` and `database/mongodb`: pass the request or command
  context to `Add` and `Observe`.
- `metrics/prometheus` and `metrics/otel`: add the context parameter to every
  counter, gauge and histogram; otel can use it for exemplars.
- `middleware/retry`: no API change; document that remote errors are retried
  only by the hop that received them.
- `broker/kafka`, `broker/nats`, `broker/rabbitmq`: implement
  `broker.ReadySubscriber` where subscription setup can fail or take time, and
  update tests expecting the consumer name `consumer:<topic>` for groups.
- `jobqueue/asynq`: rename `jobqueue.Delay`, `MaxRetry` and `Unique` to their
  `With*` forms; consider implementing `jobqueue.ReadyRunner`.
- `config/consul`, `config/etcd`, `config/nacos`, `encoding/yaml`: pass sources
  to `config.New` as a slice and use `Watch` options instead of
  `WatchWithOptions` in code, tests and READMEs.
- `discovery/consul`, `discovery/etcd`, `discovery/kubernetes`,
  `discovery/nacos`: rename httpx server options in tests; map the registry's
  weight to `Instance.Weight` where it has one.
- `selector/p2c` and other stateful selectors: prune per-node state through
  `transport.WithNodeObserver` and honor `Node.Weight`.
- servex-examples `http-server`: rename httpx options and replace `req.(*T)`
  with `httpx.RouteFunc`; `http-client` needs no change beyond the version.

### Compatibility

- Requires Go 1.27.3 and kitx v1.0.0; no new module dependencies.
- HTTP error bodies keep their fields. Servers earlier than v1.2.0 do not
  demote remote errors, so a mixed chain still multiplies retries at the hops
  running older versions.

## v1.1.1

Release date: 2026-10-27.

### Added

- `httpx.HTTPMiddleware` wraps the whole server handler with `net/http`
  middleware. It runs before routing and body binding, so authentication and
  rate limiting can reject a request before its body is read, and
  instrumentation observes unmatched routes, 413s and binding failures.
- `httpx.DefaultMaxBodyBytes` (4 MiB), the default request body limit.
- HTTP error bodies carry `retryable`, the effective `errorx` retry flag;
  clients restore it, so an explicit `WithRetryable` survives a service hop.

### Security

- `Handle` and `Route` limit request bodies to `DefaultMaxBodyBytes` by
  default. Previously bodies were unbounded unless `MaxBodyBytes` was set, so
  an unauthenticated client could make the server buffer an arbitrarily large
  body before endpoint middleware ran.

### Fixed

- An httpx client transport failure is retryable only when no connection was
  obtained or the request is replayable under `net/http` rules (GET, HEAD,
  OPTIONS, TRACE, or an `Idempotency-Key`/`X-Idempotency-Key` header).
  Connection failures after a non-idempotent request such as POST may have
  reached the node, so they are now explicitly non-retryable instead of being
  retried, possibly executing the request twice.

### Compatibility

- Servers that relied on unbounded bodies must set `MaxBodyBytes` explicitly;
  a nonpositive value still disables the limit. `HandleHTTP` handlers are
  unaffected.
- Error bodies without a `retryable` field, such as those of earlier servers,
  keep using the code's default retry policy; earlier clients ignore the field.
- Retrying a non-idempotent request after a mid-exchange connection failure
  now requires an idempotency key header or a custom retry predicate.
- Custom `http.RoundTripper`s that do not report `httptrace` connection events
  are treated as possibly having sent the request.
- Existing v1 APIs remain unchanged. Requires Go 1.27.3 and kitx v1.0.0;
  no new module dependencies.

## v1.1.0

Release date: 2026-10-25.

### Added

- The standard-library-only `cqrs` package: typed `Handler`, `Command` and
  `Query` contracts, `CommandFunc` and `QueryFunc` adapters, and explicit
  `ChainCommand` / `ChainQuery` middleware composition.
- Stable command/query operation metadata for application middleware, with
  documented ordering, cancellation, concurrency and read/write boundaries.

### Fixed

- Recheck queue cancellation after acquiring the scheduling lock, so stopping
  workers do not take an interrupted job intended for the next run.

### Documentation

- Complete the README's standalone HTTP example with module initialization,
  dependency resolution and separate server/client run instructions.
- Document typed application handlers in both READMEs and enforce their
  standard-library-only dependency boundary.

### Compatibility

- Existing v1 APIs remain unchanged. Requires Go 1.27.3 and kitx v1.0.0;
  no new module dependencies.
- `cqrs` does not add a dispatch bus, automatic transactions or retries.
  Applications own query read-only behavior and command idempotency.

## v1.0.0

Release date: 2026-10-23.

Initial release of `github.com/nostosx/servex`.

### Added

- Service contracts and reference implementations for application lifecycle,
  configuration, transport, storage and messaging, using the standard library
  and kitx.
- Validated configuration snapshots and reload failure callbacks.
- HTTP body limits, transport timeout options and custom response encoders.
- An in-memory byte-copying TTL/LRU cache and shared read-through Loader.
  Loaders require an owner context and positive timeout; `Close` cancels and
  joins outstanding loads. Waiters cancel independently, while shared work
  inherits owner context values. Cache keys must include tenant and
  authorization scope.
- Capped exponential retries for service-discovery watches and in-memory jobs.
- A common Makefile check interface, contribution and release procedures,
  security reporting guidance, bilingual READMEs and enforced dependency rules.

### Requirements

- Go 1.27.3 and the published `github.com/nostosx/kitx v1.0.0` module.
- No other external core or test dependencies; third-party integrations live
  in servex-contrib.
