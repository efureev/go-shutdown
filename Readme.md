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
- A user cleanup hook `OnDestroy(func(context.Context) error)`.
- Limiting the cleanup time via `SetTimeout(d)` (on timeout the callback
  receives a canceled context and `ErrShutdownTimeout` is returned).
- Integration with `context.Context` via `WaitContext(ctx, ...)`.
- An optional logger through the `Logger` interface.
- Manual shutdown triggering via `End()` (non-blocking, idempotent).
- A ready-to-use global instance and package-level aliases
  (`Wait`, `WaitWithLogger`, `OnDestroy`, `End`), as well as a dedicated
  instance via `New()`.

## Installation

```bash
go get -u github.com/efureev/go-shutdown
```

## Usage examples

The simplest case — wait for a termination signal:

```go
import "github.com/efureev/go-shutdown"

func main() {
    // ... start the application ...

    shutdown.Wait()
}
```

Wait for specific signals with a logger:

```go
import (
    "syscall"

    "github.com/efureev/go-shutdown"
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

    "github.com/efureev/go-shutdown"
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

A dedicated instance (recommended over the shared global state):

```go
sh := shutdown.New().
    SetTimeout(10 * time.Second).
    OnDestroy(func(ctx context.Context) error { return srv.Shutdown(ctx) })

if err := sh.Wait(); err != nil {
    log.Fatal(err)
}
```

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
