# Changelog

User-visible changes are recorded here. Published v1 releases
preserve public API compatibility following [semantic versioning](https://semver.org).

## Unreleased

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
