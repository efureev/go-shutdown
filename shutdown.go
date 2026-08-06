package shutdown

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var signalsDefault = []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT}

// Shutdown waits for OS signals (or a manual trigger via End) and runs the
// registered cleanup hooks before the program terminates.
//
// A Shutdown must be created with New. Everything an Option configures is
// immutable afterwards; Add, End, Wait and the accessors are safe to call from
// multiple goroutines. The hooks run exactly once and every caller of Wait
// receives the same error.
//
// An instance is single use: once its cleanup has finished, Wait returns that
// result immediately instead of running the hooks again.
type Shutdown struct {
	// configured by New, immutable afterwards
	log        *slog.Logger
	timeout    time.Duration
	signals    []os.Signal
	force      bool
	exit       func(code int)
	notify     func(chan<- os.Signal, ...os.Signal)
	stopNotify func(chan<- os.Signal)

	mu     sync.Mutex
	hooks  []hook
	reason Reason
	sig    os.Signal

	// ctx is canceled when the shutdown starts; it is both the broadcast to
	// waiters (Done) and the signal to the rest of the application (Context).
	ctx    context.Context
	cancel context.CancelCauseFunc

	reasonOnce sync.Once
	runOnce    sync.Once
	runErr     error
}

// New creates a Shutdown.
func New(opts ...Option) *Shutdown {
	s := &Shutdown{
		log:        slog.New(slog.DiscardHandler),
		signals:    signalsDefault,
		force:      true,
		exit:       os.Exit,
		notify:     signal.Notify,
		stopNotify: signal.Stop,
	}

	s.ctx, s.cancel = context.WithCancelCause(context.Background())

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Wait blocks until a watched signal arrives, ctx is canceled or End is
// called, then runs the cleanup hooks and returns their joined error.
//
// It is a shorthand for New(opts...).Wait(ctx), for the case where no cleanup
// hooks are needed.
func Wait(ctx context.Context, opts ...Option) error {
	return New(opts...).Wait(ctx)
}

// Wait blocks until a watched signal arrives, ctx is canceled or End is
// called, then runs the cleanup hooks and returns their joined error.
//
// Failures are wrapped in HookError, so errors.As reaches the hook that
// failed. A timeout yields an error wrapping ErrTimeout.
func (s *Shutdown) Wait(ctx context.Context) error {
	sigCh := make(chan os.Signal, 1)
	s.notify(sigCh, s.signals...)

	defer s.stopNotify(sigCh)

	select {
	case sig := <-sigCh:
		s.trigger(ReasonSignal, sig)
	case <-ctx.Done():
		s.trigger(ReasonContext, nil)
	case <-s.ctx.Done():
		// End, or another Wait, already started the shutdown.
	}

	return s.shutdown(ctx, sigCh)
}

// End triggers the shutdown manually. It is non-blocking and safe to call
// multiple times, before or after Wait.
func (s *Shutdown) End() {
	s.trigger(ReasonManual, nil)
}

// Done is closed when the shutdown starts, letting workers react without
// blocking in Wait:
//
//	select {
//	case <-sh.Done():
//		return
//	case job := <-jobs:
//		process(job)
//	}
func (s *Shutdown) Done() <-chan struct{} {
	return s.ctx.Done()
}

// Context is canceled when the shutdown starts. Its context.Cause reports
// ErrShutdown; use Reason for the detail of what triggered it.
//
// It is never canceled for any other purpose, so it is safe to derive
// request-scoped contexts from it.
func (s *Shutdown) Context() context.Context {
	return s.ctx
}

// trigger records what started the shutdown and releases everyone waiting on
// it. The first caller wins: Wait wakes the other waiters through the same
// context, and those must not relabel a shutdown that a signal or a canceled
// context had already started.
func (s *Shutdown) trigger(r Reason, sig os.Signal) {
	s.reasonOnce.Do(func() {
		s.mu.Lock()
		s.reason, s.sig = r, sig
		s.mu.Unlock()
	})

	// CancelCauseFunc is idempotent and keeps the first cause.
	s.cancel(fmt.Errorf("%w (%s)", ErrShutdown, s.Reason()))
}

// shutdown runs the cleanup hooks exactly once, no matter how many goroutines
// call Wait, and hands every caller the same error.
func (s *Shutdown) shutdown(ctx context.Context, sigCh <-chan os.Signal) error {
	s.runOnce.Do(func() {
		start := time.Now()
		s.logStart()

		stopWatch := s.watchForceQuit(sigCh)
		defer stopWatch()

		s.runErr = s.runHooks(ctx)

		if s.runErr != nil {
			s.log.Error("shutdown finished with errors",
				"duration", time.Since(start), "error", s.runErr)

			return
		}

		s.log.Info("shutdown complete", "duration", time.Since(start))
	})

	return s.runErr
}

// logStart reports the beginning of the sequence, naming the signal when there
// was one.
func (s *Shutdown) logStart() {
	attrs := []any{"reason", s.Reason()}
	if sig := s.Signal(); sig != nil {
		attrs = append(attrs, "signal", sig.String())
	}

	s.log.Info("shutdown started", attrs...)
}

// watchForceQuit terminates the process if another signal arrives while the
// cleanup hooks are running, and returns a function that stops the watch.
//
// Without it a hung hook would be uninterruptible: signal.Notify has disabled
// the default handler for these signals, so repeated Ctrl-C or SIGTERM would
// be delivered to a channel nobody reads, and the operator would be left with
// SIGKILL as the only way out.
func (s *Shutdown) watchForceQuit(sigCh <-chan os.Signal) func() {
	if !s.force {
		return func() {}
	}

	stopped := make(chan struct{})

	go func() {
		select {
		case sig := <-sigCh:
			s.log.Warn("shutdown forced, cleanup interrupted", "signal", sig.String())
			s.exit(exitCode(sig))
		case <-stopped:
		}
	}()

	return sync.OnceFunc(func() { close(stopped) })
}
