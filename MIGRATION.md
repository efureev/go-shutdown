# Migration from v2 to v3

The import path changes, so v2 keeps working until you switch:

```bash
go get github.com/efureev/go-shutdown/v3
```

```go
import "github.com/efureev/go-shutdown/v3"
```

## Why the break

Two problems could not be fixed without it:

- **The shared global instance.** `DefaultShutdown` plus the package-level
  helpers were mutable state shared by every caller. Two components each
  registering cleanup on it meant the first registration was silently lost.
- **You had to block in `Wait` to learn about the shutdown.** Workers and
  handlers had no way to observe it, which forced the whole application to be
  structured around one blocking call in `main`.

Everything else below is cleanup that came along for the ride.

## Identifier map

| v2 | v3 |
|---|---|
| `shutdown.Wait(sigs...)` | `shutdown.Wait(ctx, shutdown.WithSignals(sigs...))` |
| `shutdown.WaitContext(ctx, sigs...)` | `shutdown.Wait(ctx, shutdown.WithSignals(sigs...))` |
| `shutdown.WaitWithLogger(l, sigs...)` | `shutdown.Wait(ctx, shutdown.WithLogger(l))` |
| `shutdown.OnDestroy(fn)` / `shutdown.Add(...)` | `sh := shutdown.New(); sh.Add(name, fn)` |
| `shutdown.End()`, `shutdown.ResetHooks()` | methods on your own instance |
| `shutdown.DefaultShutdown` | removed — create an instance with `New` |
| `sh.SetLogger(l)` | `New(shutdown.WithLogger(l))` |
| `sh.SetTimeout(d)` | `New(shutdown.WithTimeout(d))` |
| `sh.SetForceOnSecondSignal(v)` | `New(shutdown.WithForceOnSecondSignal(v))` |
| `sh.Wait(sigs...)` / `sh.WaitContext(ctx, sigs...)` | `sh.Wait(ctx)`, signals via `WithSignals` |
| `sh.OnDestroy(fn)` | `sh.Add(name, fn)` |
| `sh.ResetHooks()` | removed — build the instance you want |
| `shutdown.DestroyFunc` | `shutdown.HookFunc` (same signature) |
| `shutdown.Logger` interface | `*slog.Logger` via `WithLogger` |
| `shutdown.ErrShutdownTimeout` | `shutdown.ErrTimeout` (now wraps `context.DeadlineExceeded`) |

`Reason`, `Signal()`, `ExitCode()` and `End()` are unchanged.

## Before and after

v2:

```go
shutdown.OnDestroy(func(ctx context.Context) error {
    return srv.Shutdown(ctx)
})

if err := shutdown.WaitWithLogger(logger, syscall.SIGTERM); err != nil {
    log.Fatal(err)
}
```

v3:

```go
sh := shutdown.New(
    shutdown.WithLogger(logger),          // *slog.Logger
    shutdown.WithSignals(syscall.SIGTERM),
)
sh.Add("http", func(ctx context.Context) error {
    return srv.Shutdown(ctx)
})

if err := sh.Wait(context.Background()); err != nil {
    log.Fatal(err)
}
```

## Behavior changes to check

- **Configuration is fixed at construction.** The `Set*` chain is gone; a
  running `Wait` can no longer be reconfigured underneath.
- **Signals belong to the instance,** not to the `Wait` call.
- **Hooks are named,** and failures come back as `*HookError`. Code that
  compared the returned error by identity should use `errors.Is`/`errors.As`.
- **The logger is silent by default.** v2 also logged nothing without a
  logger, but the messages themselves changed: they are now structured
  records (`slog`), not the `shutdown started...` strings.
- **`ErrShutdownTimeout` became `ErrTimeout`** and now wraps
  `context.DeadlineExceeded`; the timeout error also names the hooks that were
  still running.

## What did not change

Zero dependencies. LIFO teardown order. A failing hook does not stop the
others. The force quit on a second signal, on by default. `ExitCode()`
following the `128+signum` convention. Hooks receive a context detached from
the one you waited on, so cancelling that context starts the cleanup instead
of aborting it.
