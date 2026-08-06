# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Because Go encodes the major version in the import path, every major release
also changes the module path.

## [Unreleased]

## [3.0.0]

Full redesign. The import path becomes `github.com/efureev/go-shutdown/v3`, so
v2 keeps working until you switch. Step-by-step upgrade instructions are in
[MIGRATION.md](MIGRATION.md).

Two problems could not be fixed without breaking the API, and they are what
justified the major version: the shared global instance silently lost cleanup
registered by other components, and there was no way to observe the shutdown
without blocking in `Wait`.

### Added

- `Done()` and `Context()` — workers react to the shutdown without anyone
  calling `Wait`. `context.Cause` of that context is `ErrShutdown`.
- `Parallel()` — consecutive hooks marked with it run as one concurrent group;
  a plain hook is a barrier between groups.
- `HookTimeout(d)` — a budget for a single hook, independent of the global one.
- `HookError` — a typed error carrying the hook name, so `errors.As` reaches
  the subsystem that failed.
- `Option` constructors: `WithLogger`, `WithTimeout`, `WithSignals`,
  `WithForceOnSecondSignal`.
- `Wait(ctx, opts...)` at package level — the one-liner, without shared state.
- A timeout now names the hooks that were still running.

### Changed

- Configuration moved from chained setters to constructor options and is
  immutable afterwards, so it can no longer race with a running `Wait`.
- `Wait` and `WaitContext` collapsed into `Wait(ctx)`; signals moved from the
  call to `WithSignals`.
- Logging goes through `*slog.Logger` and is structured. Cleanup failures are
  reported at error level — previously they were not logged at all.
- `ErrShutdownTimeout` became `ErrTimeout` and now wraps
  `context.DeadlineExceeded`.
- `DestroyFunc` renamed to `HookFunc` (same signature).
- Package split into `shutdown.go`, `hooks.go`, `options.go`, `reason.go` and
  `errors.go`.

### Removed

- `DefaultShutdown` and every package-level helper built on it
  (`WaitContext`, `WaitWithLogger`, `OnDestroy`, `Add`, `ResetHooks`, `End`).
- The `Logger` interface, replaced by `*slog.Logger`.
- `OnDestroy` and `ResetHooks` methods — `Add` is the only registration point.
- `SetLogger`, `SetTimeout`, `SetForceOnSecondSignal` — see options above.

### Fixed

- Cleanup is no longer handed an already-canceled context when no timeout is
  configured; a context-triggered shutdown used to abort `srv.Shutdown(ctx)`
  immediately.
- Tests no longer send real signals to the test process, removing a class of
  flakiness and the risk of killing the whole test binary.

## [2.0.1]

Cleanup of the v2 line: context handling in `WaitContext`, timeout behavior
documentation, and build tooling.

## [2.0.0]

`OnDestroy` gained a `context.Context` parameter and an `error` result,
`SetTimeout` and `WaitContext` were introduced, and the module moved to
`/v2`.

## [1.3.2] and earlier

See the [releases page](https://github.com/efureev/go-shutdown/releases).

[Unreleased]: https://github.com/efureev/go-shutdown/compare/v3.0.0...HEAD
[3.0.0]: https://github.com/efureev/go-shutdown/compare/v2.0.1...v3.0.0
[2.0.1]: https://github.com/efureev/go-shutdown/compare/v2.0.0...v2.0.1
[2.0.0]: https://github.com/efureev/go-shutdown/compare/v1.3.2...v2.0.0
[1.3.2]: https://github.com/efureev/go-shutdown/releases/tag/v1.3.2
