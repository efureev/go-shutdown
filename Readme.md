[![Test](https://github.com/efureev/go-shutdown/actions/workflows/test.yml/badge.svg)](https://github.com/efureev/go-shutdown/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/efureev/go-shutdown.svg)](https://pkg.go.dev/github.com/efureev/go-shutdown)
[![Go Report Card](https://goreportcard.com/badge/github.com/efureev/go-shutdown)](https://goreportcard.com/report/github.com/efureev/go-shutdown)

# Shutdown

> Read this in other languages: [Русский](Readme.ru.md)

`go-shutdown` is a small package for **graceful shutdown** of Go applications
and services.

It blocks execution and waits for operating system signals (by default
`SIGINT`, `SIGTERM`, `SIGQUIT`), and when one is received it runs your cleanup
function (closing connections, stopping workers, flushing buffers, etc.) before
the process exits.

## Features

- Waiting for standard or custom OS signals.
- Any number of cleanup hooks: `Add(name, func(context.Context) error)` and
  the unnamed `OnDestroy(func(context.Context) error)`. Hooks run in reverse
  registration order (LIFO), every hook runs even if an earlier one failed,
  and the errors are joined and returned.
- Limiting the cleanup time via `SetTimeout(d)` — one budget for the whole
  hook sequence (on timeout the hooks receive a canceled context and
  `ErrShutdownTimeout` is returned).
- Integration with `context.Context` via `WaitContext(ctx, ...)`. The context
  handed to the hooks is detached from it, so cancelling the context you wait
  on starts the cleanup instead of aborting it.
- A force quit: a signal arriving while the cleanup is still running
  terminates the process with exit code `128+signum`, so a hung hook can be
  interrupted. Opt out with `SetForceOnSecondSignal(false)`.
- Introspection of what triggered the shutdown: `Reason()` (`ReasonSignal`,
  `ReasonContext`, `ReasonManual`), `Signal()` and `ExitCode()` following the
  `128+signum` convention.
- An optional logger through the `Logger` interface.
- Manual shutdown triggering via `End()` (non-blocking, idempotent).
- A ready-to-use global instance and package-level aliases
  (`Wait`, `WaitWithLogger`, `OnDestroy`, `Add`, `End`), as well as a
  dedicated instance via `New()`.

A `Shutdown` runs its hooks once. Calling `Wait` again on an instance whose
cleanup already finished returns that same error immediately, and concurrent
`Wait` callers all receive the result of the single cleanup run.

## Upgrading from v2.0.x

**`OnDestroy` now appends** a hook instead of replacing the previously
registered one. Previously a second call silently dropped the first callback,
which lost cleanup whenever two components shared `DefaultShutdown`. If you
relied on the replacing behavior, call `ResetHooks()` first.

**A signal received during cleanup now terminates the process.** Previously it
was swallowed, which left a hung hook interruptible only by `SIGKILL`. If your
cleanup must never be interrupted, call `SetForceOnSecondSignal(false)`.

**A repeated `Wait` no longer blocks forever.** It returns the error of the
cleanup that already ran. Note that this makes an instance single-use: reusing
one across several shutdown cycles (in tests, for example) needs a fresh
instance from `New()`.

## Installation

```bash
go get -u github.com/efureev/go-shutdown/v2
```

## Usage examples

The simplest case — wait for a termination signal:

```go
import "github.com/efureev/go-shutdown/v2"

func main() {
    // ... start the application ...

    shutdown.Wait()
}
```

Wait for specific signals with a logger:

```go
import (
    "syscall"

    "github.com/efureev/go-shutdown/v2"
)

func main() {
    // ... start the application ...

    shutdown.WaitWithLogger(logger, syscall.SIGINT, syscall.SIGTERM)
}
```

With a cleanup function and a logger (the callback receives a
`context.Context` and returns an `error`):

```go
import (
    "context"

    "github.com/efureev/go-shutdown/v2"
)

func main() {
    // ... start the application ...

    err := shutdown.
        OnDestroy(func(ctx context.Context) error {
            return module.processing.EndJobListen(ctx)
        }).
        SetLogger(module.Log()).
        Wait()
    if err != nil {
        // handle cleanup error
    }
}
```

Several subsystems, torn down in the opposite order they were started:

```go
sh := shutdown.New().SetTimeout(15 * time.Second)

sh.Add("http", func(ctx context.Context) error { return srv.Shutdown(ctx) })
sh.Add("consumer", func(ctx context.Context) error { return consumer.Stop(ctx) })
sh.Add("db", func(context.Context) error { return db.Close() })

// On shutdown: db → consumer → http. A failing hook does not stop the rest;
// err joins every failure, each annotated with its hook name.
if err := sh.Wait(); err != nil {
    log.Printf("shutdown finished with errors: %v", err)
}
```

A dedicated instance (recommended over the shared global state):

```go
sh := shutdown.New().
    SetTimeout(10 * time.Second).
    OnDestroy(func(ctx context.Context) error { return srv.Shutdown(ctx) })

if err := sh.Wait(); err != nil {
    log.Fatal(err)
}
```

Reporting why the process stopped, and exiting accordingly:

```go
sh := shutdown.New().OnDestroy(func(ctx context.Context) error { return srv.Shutdown(ctx) })

if err := sh.Wait(); err != nil {
    log.Printf("cleanup failed: %v", err)
}

// reason=signal signal=terminated code=143
log.Printf("reason=%s signal=%v code=%d", sh.Reason(), sh.Signal(), sh.ExitCode())

os.Exit(sh.ExitCode())
```

`Signal()` returns `nil` unless `Reason()` is `ReasonSignal`, and `ExitCode()`
is `0` for every reason other than a signal. Cleanup errors do not affect the
exit code — that call is yours.

Stop on a signal or on the cancellation of an external context:

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

if err := shutdown.New().WaitContext(ctx); err != nil {
    log.Fatal(err)
}
```

## License

See the [LICENSE](LICENSE) file.
