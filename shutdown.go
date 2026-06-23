package shutdown

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var signalsDefault = []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT}

// ErrShutdownTimeout is returned by Wait/WaitContext when the OnDestroy
// function did not complete within the configured timeout.
var ErrShutdownTimeout = errors.New("shutdown: destroy function timeout")

// DestroyFunc is the cleanup callback executed during shutdown.
//
// The provided context is canceled when the configured timeout (see
// SetTimeout) elapses or when the context passed to WaitContext is canceled,
// allowing the callback to abort long-running work gracefully.
type DestroyFunc func(ctx context.Context) error

// Shutdown waits for OS signals (or a manual trigger via End) and runs an
// optional cleanup callback before the program terminates.
//
// A Shutdown must be created with New. It is safe to configure it (SetLogger,
// SetTimeout, OnDestroy) and call End from multiple goroutines. Wait/WaitContext
// is intended to be called once per instance.
type Shutdown struct {
	mu          sync.Mutex
	log         Logger
	timeout     time.Duration
	onDestroyFn DestroyFunc

	done chan struct{}
}

// DefaultShutdown is the package-wide instance used by the package-level
// helpers (Wait, WaitWithLogger, OnDestroy, End).
//
// It is mutable global state shared across all callers: configuring it from
// independent components may lead to surprising interactions. Prefer a
// dedicated instance created with New for anything beyond a simple main().
var DefaultShutdown = New()

// New creates a new Shutdown instance.
func New() *Shutdown {
	return &Shutdown{
		done: make(chan struct{}, 1),
	}
}

// Wait blocks until one of the given signals is received (defaults to
// SIGINT, SIGTERM and SIGQUIT) or End is called, then runs the OnDestroy
// callback and returns its error.
func (s *Shutdown) Wait(signals ...os.Signal) error {
	return s.WaitContext(context.Background(), signals...)
}

// WaitContext behaves like Wait but also returns when the provided context
// is canceled. The same context (optionally bounded by SetTimeout) is passed
// to the OnDestroy callback so it can react to cancellation.
func (s *Shutdown) WaitContext(ctx context.Context, signals ...os.Signal) error {
	if len(signals) == 0 {
		signals = signalsDefault
	}

	sigCtx, stop := signal.NotifyContext(ctx, signals...)
	defer stop()

	select {
	case <-sigCtx.Done():
	case <-s.done:
	}

	logInfo(s.logger(), `shutdown started...`)

	err := s.runOnDestroy(ctx)

	logTrace(s.logger(), `shutdown complete...`)

	return err
}

// runOnDestroy executes the user destroy function (if any).
//
// When a timeout is configured the function runs in a separate goroutine with
// a context bounded by the timeout; ErrShutdownTimeout is returned if it does
// not finish in time. The context is canceled in that case so the callback
// can stop its work.
func (s *Shutdown) runOnDestroy(ctx context.Context) error {
	s.mu.Lock()
	fn := s.onDestroyFn
	timeout := s.timeout
	s.mu.Unlock()

	if fn == nil {
		return nil
	}

	if timeout <= 0 {
		return fn(ctx)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resCh := make(chan error, 1)
	go func() { resCh <- fn(ctx) }()

	select {
	case err := <-resCh:
		return err
	case <-ctx.Done():
		return ErrShutdownTimeout
	}
}

// SetLogger sets the logger used to report shutdown progress.
func (s *Shutdown) SetLogger(l Logger) *Shutdown {
	s.mu.Lock()
	s.log = l
	s.mu.Unlock()

	return s
}

// SetTimeout limits the time allowed for the OnDestroy callback to complete.
// A non-positive duration (the default) means no timeout.
func (s *Shutdown) SetTimeout(d time.Duration) *Shutdown {
	s.mu.Lock()
	s.timeout = d
	s.mu.Unlock()

	return s
}

// OnDestroy registers the callback executed when the app/service is
// terminating.
func (s *Shutdown) OnDestroy(fn DestroyFunc) *Shutdown {
	s.mu.Lock()
	s.onDestroyFn = fn
	s.mu.Unlock()

	return s
}

// End triggers the shutdown manually. It is non-blocking and safe to call
// multiple times, before or after Wait.
func (s *Shutdown) End() {
	select {
	case s.done <- struct{}{}:
	default:
	}
}

// logger returns the configured logger under the lock.
func (s *Shutdown) logger() Logger {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.log
}

// Wait is a helper that waits on DefaultShutdown.
func Wait(signals ...os.Signal) error {
	return DefaultShutdown.Wait(signals...)
}

// WaitContext is a helper that waits on DefaultShutdown with a context.
func WaitContext(ctx context.Context, signals ...os.Signal) error {
	return DefaultShutdown.WaitContext(ctx, signals...)
}

// WaitWithLogger configures the logger on DefaultShutdown and waits.
func WaitWithLogger(logger Logger, signals ...os.Signal) error {
	return DefaultShutdown.SetLogger(logger).Wait(signals...)
}

// End triggers a manual shutdown of DefaultShutdown.
func End() {
	DefaultShutdown.End()
}

// OnDestroy registers the destroy callback on DefaultShutdown.
func OnDestroy(fn DestroyFunc) *Shutdown {
	return DefaultShutdown.OnDestroy(fn)
}

func logTrace(logger Logger, args ...any) {
	if logger != nil {
		logger.Trace(args...)
	}
}

func logInfo(logger Logger, args ...any) {
	if logger != nil {
		logger.Info(args...)
	}
}
