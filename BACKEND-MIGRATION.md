# Backend migration (unreleased)

This source change moves concrete backends, algorithms and reference
implementations out of servex. The second phase also moves the shared client
engine and Cache Loader. App lifecycle, health Group, registration and thin
Consumer/Worker runtime adapters intentionally remain in core. Public installation requires a coordinated core/contrib
release; all manifests remain free of local replacements.

| Previous core API/path | Contrib API/path |
|---|---|
| transport/httpx | servex-contrib/transport/httpx |
| storage/database | servex-contrib/database/sql (package database) |
| storage/cache/memory | servex-contrib/cache/memory |
| discovery/memory | servex-contrib/discovery/memory |
| messaging/broker/memory | servex-contrib/broker/memory |
| messaging/jobqueue/memory | servex-contrib/jobqueue/memory |
| encoding.JSON | servex-contrib/encoding/json.Codec |
| selector.Random() | servex-contrib/selector/random.New() |
| metrics.Noop() | servex-contrib/metrics/noop.New() |
| tracing.Noop()/NoopPropagator() | servex-contrib/tracing/noop.New()/NewPropagator() |
| transport.Client/NewClient/ClientOption | servex-contrib/transport/client.Client/New/Option |
| storage/cache.Loader/NewLoader/LoaderOption | servex-contrib/cache/loader.Loader/New/Option |

Paths in the right column start with github.com/nostosx/.
The first phase adds nine independent modules; the second adds cache/loader
and transport/client. Random is a package in the existing
selector module. Backend-dependent core tests move with their implementations.
Core tests continue to depend only on standard library, core and kitx.

## Explicit shared-runtime dependencies

The shared client now lives in contrib and requires an explicit selector:

```go
client, err := sxclient.New(ctx, target,
    sxclient.WithSelector(random.New()),
)
```

Omitting the selector returns an error before the client starts. HTTP/gRPC
adapters choose their defaults in contrib; custom selectors remain supported.

All client With* options move to the new package, as do Client and Option.
Core transport retains Server, Invoker and InvokerFunc; it contains no target
resolution, node snapshots, watch recovery or invocation orchestration.

Cache Loader requires an explicit codec and reports cacheloader.ErrCodecRequired
when none is supplied:

```go
loader, err := cacheloader.New[User](owner, backend, timeout,
    cacheloader.WithCodec(jsoncodec.Codec),
)
```

Its timeout, cancellation, isolated waiters, cache miss/error and write-back
semantics remain covered by the original regression suite. Constructor tests
use an explicit test codec, not a production default.

Import sxclient from github.com/nostosx/servex-contrib/transport/client and
cacheloader from github.com/nostosx/servex-contrib/cache/loader. WithTTL,
WithCodec and ErrLoaderClosed/ErrInvalidLoadTimeout/ErrCodecRequired move with
Loader. Core cache keeps only Cache and ErrNotFound. There are no forwarding
aliases and no replacement Client/Loader interfaces without an actual consumer.

## Minimal core runtime

App owns lifecycle order, rollback, drain, stop deadlines and runtime failures.
health.Group composes Checker implementations. Consumer/Worker adapt Subscriber
and Runner to Hook, metadata, middleware and readiness; they do not implement
broker acknowledgement, task scheduling or retry engines. Registration
lifecycle helpers also remain in core. These are explicit framework runtime
responsibilities, not deferred backend extractions. New backend and execution
policies belong in contrib, even when their implementation uses only stdlib.

## Tracing context

tracing.SpanFromContext returns nil for an absent Span, nil Context or an
explicit nil Span shadowing a parent. It never chooses a no-op backend.
Callers check for nil before reading IDs or recording events. Explicit
no-op tracer/propagator construction belongs to contrib. The no-parent
no-op path retains its no-allocation regression.

## Dependency ownership

Contrib modules may explicitly depend on foundational contrib implementations
such as JSON, selectors and no-op telemetry. Package cycles and imports from
core to contrib remain forbidden. Generic middleware's only additional
dependencies are the two no-op telemetry modules; it does not gain SDK
dependencies. Consumers keep ownership of explicit SDK/transport resources.

Published baseline versions in manifests are not evidence that these new APIs
are available from the public proxy. Development validation uses throwaway
modfiles pointing at matching source snapshots. Release core first, then
foundational backends, dependent adapters and consumers; refresh exact versions
and published checksums before public consumer verification.
