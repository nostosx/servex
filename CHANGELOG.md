# Changelog

User-visible changes are recorded here. Published v1 releases
preserve public API compatibility following [semantic versioning](https://semver.org).

## Unreleased

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
