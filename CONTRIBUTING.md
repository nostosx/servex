# Contributing

## Setup

Use Go 1.27.3, GNU Make and a shell available to Make. Race tests require a
supported platform and a working C toolchain. Install tools outside this
module using Go 1.27.3. kitx v1.0.0 resolves from the Go module proxy;
no sibling kitx checkout is needed.

```sh
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
```

Put the selected Go binary and tools on `PATH`. Do not change global Go
configuration to work around a checkout-specific toolchain problem.

## Checks

Run from the repository root:

```sh
make check
make lint
make vulncheck
```

The Makefile exports `GOWORK=off`. `check` rejects formatting, modernizer and
module-tidy changes, vets and builds normal and `integration` configurations,
checks the standard-library and kitx dependency boundary, then runs shuffled race tests
without test-result caching. `lint` and `vulncheck` also inspect both build
configurations. No command in this check sequence starts external services.

`make fmt`, `make fix` and `make tidy` modify files explicitly; inspect their
changes before rerunning checks. `GO`, `GOFMT`, `GOLANGCI_LINT` and `GOVULNCHECK`
can select installed executables without changing module dependencies.
The core has no service-backed integration tests.

## Changes

Keep the core limited to the standard library and kitx, including tests. Read
the dependency boundaries below before adding a package or dependency;
update the documented table and `internal/archtest` together. Place further
implementations and third-party SDKs in servex-contrib, and runnable programs
in servex-examples.

Use clear Go names and English comments that explain contracts or reasons.
Add a focused regression test for a behavior fix. Tests must be deterministic,
release their resources and use explicit synchronization rather than timing
assumptions. Preserve cancellation and error identity across API boundaries.

Describe the problem, resulting behavior and checks in a pull request. Add
user-visible changes to [CHANGELOG.md](CHANGELOG.md); internal refactoring
does not need a release-note entry. Report sensitive issues through
[SECURITY.md](SECURITY.md). See [RELEASING.md](RELEASING.md) for release gates.

## Dependency boundaries

The core owns protocol-independent contracts and one reference implementation
per contract. Additional implementations and SDKs belong in servex-contrib;
runnable applications belong in servex-examples. Logging keeps `log/slog` and
SQL access keeps `database/sql`. Constructors are wired explicitly, without
registries, DI containers or global mutable state.

### Allowed imports

Paths are relative to `github.com/nostosx/servex`. Each row is the complete set
of core packages that a package's non-test files may import. Standard-library
imports are always allowed; entries do not grant access to subpackages or
transitive imports.

| Package | Allowed core imports |
|---|---|
| `endpoint` | none |
| `metadata` | none |
| `errorx` | none |
| `health` | none |
| `encoding` | none |
| `observability/metrics` | none |
| `observability/tracing` | none |
| `app` | none |
| `internal/host` | none |
| `internal/runhook` | none |
| `config` | `encoding` |
| `config/file` | `config`, `encoding` |
| `selector` | `errorx` |
| `discovery` | `app`, `internal/host` |
| `discovery/memory` | `discovery` |
| `storage/cache` | `encoding` |
| `storage/cache/memory` | `storage/cache` |
| `storage/database` | `app`, `observability/metrics`, `observability/tracing` |
| `messaging/broker` | `app`, `endpoint`, `metadata`, `errorx`, `internal/runhook` |
| `messaging/broker/memory` | `messaging/broker`, `endpoint`, `errorx`, `metadata` |
| `messaging/jobqueue` | `app`, `endpoint`, `metadata`, `errorx`, `internal/runhook` |
| `messaging/jobqueue/memory` | `messaging/jobqueue`, `endpoint`, `errorx`, `metadata` |
| `transport` | `app`, `endpoint`, `metadata`, `errorx`, `selector`, `discovery` |
| `transport/httpx` | `transport`, `app`, `endpoint`, `metadata`, `errorx`, `encoding`, `health`, `selector`, `discovery` |
| `internal/archtest` | any core package |

Approved external imports are `kitx/backoff` for `transport` and
`messaging/jobqueue/memory`, `kitx/singleflight` for `storage/cache`, and
`kitx/ttlcache` for `storage/cache/memory`, under `github.com/nostosx/`.
Tests may use any core package as a fixture. All source and tests must depend
only on the standard library, this module and kitx; `golang.org/x` packages
count as third party. kitx is the sole external module and must remain
standard-library-only.

[`internal/archtest`](internal/archtest/archtest_test.go) rejects undocumented
packages, forbidden imports and stale table entries. Update its `allowed` and
`allowedKitx` tables with this section when boundaries change. `make deps-check`
checks module and package graphs; `make check` includes it and architecture tests.
Contrib modules import public core packages and never each other. Their
`middleware` and `selector` modules share the standard-library, servex and kitx
boundary; other modules own their SDK dependencies.

### Ownership and concurrency

Reuse kitx where behavior and ownership agree. Discovery-watch and in-memory
job retries share exponential backoff; recovery and retry policy remain in
servex. The in-memory cache adapts kitx/ttlcache and retains byte copies, error
mapping, the 10,000-entry default, unlimited-capacity options and clock injection.
Stored slices stay private and immutable.

Cache Loader adapts kitx/singleflight.SharedGroup. Its owner context and
positive timeout bound shared work; `Close` cancels and joins it without
closing the backend. Waiters cancel independently. Keys must include tenant
and authorization scope; shared credentials come from explicit callback
dependencies or the owner context. Same-key callbacks must be interchangeable
and shared mutable results must be treated as immutable. Backends and callbacks
must honor cancellation. Applications must bound distinct active keys.
Callbacks must not call `Close` or reenter their own key. The adapter owns
decoding, cache-error fallback, cache rechecking and write-back policy.

`internal/host` owns discovery address selection, `internal/runhook` owns
consumer lifecycle handling, and `internal/archtest` enforces dependency rules.
These responsibilities remain private to servex.
