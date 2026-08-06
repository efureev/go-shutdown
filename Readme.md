[![Test](https://github.com/efureev/go-shutdown/actions/workflows/test.yml/badge.svg)](https://github.com/efureev/go-shutdown/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/efureev/go-shutdown/v3.svg)](https://pkg.go.dev/github.com/efureev/go-shutdown/v3)
[![Go Report Card](https://goreportcard.com/badge/github.com/efureev/go-shutdown/v3)](https://goreportcard.com/report/github.com/efureev/go-shutdown/v3)

# Shutdown

> Read this in other languages: [Русский](Readme.ru.md)

`go-shutdown` is a small, dependency-free package for **graceful shutdown** of
Go applications and services.

It waits for OS signals (by default `SIGINT`, `SIGTERM`, `SIGQUIT`), for the
cancellation of a context, or for a manual trigger, and then runs your cleanup
hooks — closing connections, stopping workers, flushing buffers — before the
process exits.

## Features

- **Any number of named cleanup hooks.** They run in reverse registration
  order (LIFO), so subsystems are torn down in the opposite order they were
  brought up.
- **Parallel groups.** Consecutive hooks marked `Parallel()` run together; a
  plain hook is a barrier between groups.
- **Timeouts at both levels.** `WithTimeout` bounds the whole sequence,
  `HookTimeout` bounds one hook. A timeout names the hooks still running.
- **Every hook runs even if an earlier one failed.** Failures are joined and
  wrapped in `HookError`, so `errors.As` reaches the one that failed.
- **Observability without blocking.** `Done()` and `Context()` let workers
  react to the shutdown without anyone calling `Wait`.
- **A force quit.** A signal arriving while cleanup is running terminates the
  process with exit code `128+signum`, so a hung hook can be interrupted.
- **Introspection.** `Reason()`, `Signal()` and `ExitCode()`.
- **Structured logging** through `*slog.Logger`. Silent by default.
- **Zero dependencies.** Standard library only.

## Installation

```bash
go get -u github.com/efureev/go-shutdown/v3
```

## Usage

The simplest case — wait for a termination signal, no cleanup:

```go
import (
    "context"

    "github.com/efureev/go-shutdown/v3"
)

func main() {
    // ... start the application ...

    if err := shutdown.Wait(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```

Several subsystems, torn down in the opposite order they were started:

```go
sh := shutdown.New(
    shutdown.WithTimeout(15*time.Second),
    shutdown.WithLogger(slog.Default()),
)

sh.Add("db", func(context.Context) error { return db.Close() })
sh.Add("cache", func(ctx context.Context) error { return cache.Flush(ctx) }, shutdown.Parallel())
sh.Add("search", func(ctx context.Context) error { return search.Flush(ctx) }, shutdown.Parallel())
sh.Add("http", func(ctx context.Context) error { return srv.Shutdown(ctx) })

// teardown: http, then cache and search together, then db
if err := sh.Wait(context.Background()); err != nil {
    log.Printf("cleanup failed: %v", err)
}

os.Exit(sh.ExitCode())
```

Workers react to the shutdown without blocking in `Wait`:

```go
for {
    select {
    case <-sh.Done():
        return
    case job := <-jobs:
        process(job)
    }
}
```

`sh.Context()` is the same signal as a `context.Context`, so it can be passed
down or derived from. Its `context.Cause` is `ErrShutdown`.

Finding out which hook failed:

```go
var hookErr *shutdown.HookError
if errors.As(sh.Wait(ctx), &hookErr) {
    log.Printf("subsystem %q failed to stop: %v", hookErr.Hook, hookErr.Err)
}
```

Giving one slow subsystem its own budget:

```go
sh.Add("drain", drainQueue, shutdown.HookTimeout(5*time.Second))
```

On expiry `Wait` returns an error wrapping `ErrTimeout` (and therefore
`context.DeadlineExceeded`) that names the hooks still running. A hook that
ignores its context keeps running in its own goroutine, so long cleanup must
honor `ctx` to be interruptible.

## Stopping manually

`End()` starts the shutdown from code. It is non-blocking, idempotent, and may
be called before or after `Wait`.

An instance is single use: once its cleanup has finished, `Wait` returns that
same result immediately, and concurrent callers of `Wait` all receive it.

## Upgrading from v2

The API changed substantially — see [MIGRATION.md](MIGRATION.md).

## License

See the [LICENSE](LICENSE) file.
