package shutdown

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var signalsDefault = []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT}

// ErrShutdownTimeout is returned by Wait/WaitContext when the cleanup hooks
// did not complete within the configured timeout.
var ErrShutdownTimeout = errors.New("shutdown: destroy function timeout")

// DestroyFunc is the cleanup callback executed during shutdown.
//
// The provided context is canceled when the configured timeout (see
// SetTimeout) elapses, allowing the callback to abort long-running work
// gracefully. It is deliberately detached from the context passed to
// WaitContext: canceling that context is what starts the shutdown, so it
// must not abort the cleanup it just triggered.
type DestroyFunc func(ctx context.Context) error

// Reason tells what triggered the shutdown.
type Reason int

const (
	// ReasonNone means the shutdown has not been triggered yet.
	ReasonNone Reason = iota
	// ReasonSignal means one of the watched OS signals was received.
	ReasonSignal
	// ReasonContext means the context passed to WaitContext was canceled.
	ReasonContext
	// ReasonManual means End was called.
	ReasonManual
)

// String implements fmt.Stringer.
func (r Reason) String() string {
	switch r {
	case ReasonNone:
		return "none"
	case ReasonSignal:
		return "signal"
	case ReasonContext:
		return "context"
	case ReasonManual:
		return "manual"
	default:
		return "unknown"
	}
}

// hook is a cleanup callback together with the name it is reported under.
type hook struct {
	name string
	fn   DestroyFunc
}

// wrap annotates an error with the hook name, so a failure in one of several
// hooks can be attributed. Unnamed hooks (registered via OnDestroy) are
// returned untouched to keep the error identity the caller provided.
func (h hook) wrap(err error) error {
	if h.name == "" {
		return err
	}

	return fmt.Errorf("shutdown: hook %q: %w", h.name, err)
}

// Shutdown waits for OS signals (or a manual trigger via End) and runs the
// registered cleanup hooks before the program terminates.
//
// A Shutdown must be created with New. It is safe to configure it (SetLogger,
// SetTimeout, OnDestroy, Add) and to call End and Wait/WaitContext from
// multiple goroutines: the hooks run exactly once and every caller of
// Wait/WaitContext receives the same error.
type Shutdown struct {
	mu      sync.Mutex
	log     Logger
	timeout time.Duration
	hooks   []hook
	force   bool
	exit    func(code int)
	reason  Reason
	sig     os.Signal

	endOnce sync.Once
	done    chan struct{}

	reasonOnce sync.Once

	runOnce sync.Once
	runErr  error
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
		done:  make(chan struct{}),
		force: true,
		exit:  os.Exit,
	}
}

// Wait blocks until one of the given signals is received (defaults to
// SIGINT, SIGTERM and SIGQUIT) or End is called, then runs the registered
// cleanup hooks and returns their error.
//
// Calling Wait again on an instance whose cleanup already finished returns
// that same error immediately instead of blocking.
func (s *Shutdown) Wait(signals ...os.Signal) error {
	return s.WaitContext(context.Background(), signals...)
}

// WaitContext behaves like Wait but also returns when the provided context
// is canceled.
//
// The context handed to the hooks is detached from ctx and bounded only by
// SetTimeout: ctx being canceled is one of the ways to trigger the shutdown,
// so propagating that cancellation would abort the cleanup before it starts.
func (s *Shutdown) WaitContext(ctx context.Context, signals ...os.Signal) error {
	if len(signals) == 0 {
		signals = signalsDefault
	}

	// Deliberately not signal.NotifyContext: the raw channel stays readable
	// while the hooks run, which is what makes the force quit below possible.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, signals...)

	defer signal.Stop(sigCh)

	select {
	case sig := <-sigCh:
		s.setReason(ReasonSignal, sig)
	case <-ctx.Done():
		s.setReason(ReasonContext, nil)
	case <-s.done:
		s.setReason(ReasonManual, nil)
	}

	// Release every other Wait/WaitContext on this instance; they will block
	// on runOnce below and return the same error.
	s.End()

	return s.shutdown(ctx, sigCh)
}

// setReason records what triggered the shutdown. The first writer wins: the
// End below wakes every other waiter, and those must not relabel a shutdown
// that a signal or a canceled context had already started.
func (s *Shutdown) setReason(r Reason, sig os.Signal) {
	s.reasonOnce.Do(func() {
		s.mu.Lock()
		s.reason, s.sig = r, sig
		s.mu.Unlock()
	})
}

// Reason reports what triggered the shutdown, or ReasonNone while it has not
// been triggered yet.
func (s *Shutdown) Reason() Reason {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.reason
}

// Signal reports the OS signal that triggered the shutdown. It returns nil
// when the shutdown was triggered by End, by a canceled context, or not yet
// at all — check Reason to tell those apart.
func (s *Shutdown) Signal() os.Signal {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.sig
}

// ExitCode reports the exit code matching how the shutdown was triggered:
// 128+signum for a signal, which is what a shell reports for a process killed
// by it, and 0 for every other reason.
//
// Cleanup errors are not taken into account: whether a failed hook should
// change the exit status is the caller's decision.
//
//	err := sh.Wait()
//	if err != nil {
//		log.Printf("cleanup failed: %v", err)
//	}
//	os.Exit(sh.ExitCode())
func (s *Shutdown) ExitCode() int {
	s.mu.Lock()
	sig := s.sig
	s.mu.Unlock()

	if sig == nil {
		return 0
	}

	return exitCode(sig)
}

// shutdown runs the cleanup hooks exactly once, no matter how many goroutines
// call Wait/WaitContext, and hands every caller the same error.
func (s *Shutdown) shutdown(ctx context.Context, sigCh <-chan os.Signal) error {
	s.runOnce.Do(func() {
		logInfo(s.logger(), `shutdown started...`)

		stopWatch := s.watchForceQuit(sigCh)
		defer stopWatch()

		s.runErr = s.runOnDestroy(ctx)

		logTrace(s.logger(), `shutdown complete...`)
	})

	return s.runErr
}

// watchForceQuit terminates the process if another signal arrives while the
// cleanup hooks are running, and returns a function that stops the watch.
//
// Without it a hung hook would be uninterruptible: signal.Notify has disabled
// the default handler for these signals, so repeated Ctrl-C or SIGTERM would
// be delivered to a channel nobody reads, and the operator would be left with
// SIGKILL as the only way out.
func (s *Shutdown) watchForceQuit(sigCh <-chan os.Signal) func() {
	s.mu.Lock()
	enabled, exit := s.force, s.exit
	s.mu.Unlock()

	if !enabled {
		return func() {}
	}

	stopped := make(chan struct{})

	go func() {
		select {
		case sig := <-sigCh:
			logInfo(s.logger(), `shutdown forced, cleanup interrupted...`)
			exit(exitCode(sig))
		case <-stopped:
		}
	}()

	return sync.OnceFunc(func() { close(stopped) })
}

// exitCode maps a signal to the conventional shell exit code 128+signum, so
// an orchestrator can tell a forced stop from a clean one.
func exitCode(sig os.Signal) int {
	if s, ok := sig.(syscall.Signal); ok && s > 0 {
		return 128 + int(s)
	}

	return 1
}

// runOnDestroy executes the registered cleanup hooks (if any).
//
// When a timeout is configured the hooks run in a separate goroutine with a
// context bounded by the timeout; ErrShutdownTimeout is returned if they do
// not finish in time. The context is canceled in that case so the hooks can
// stop their work.
func (s *Shutdown) runOnDestroy(ctx context.Context) error {
	s.mu.Lock()
	hooks := make([]hook, len(s.hooks))
	copy(hooks, s.hooks)
	timeout := s.timeout
	s.mu.Unlock()

	if len(hooks) == 0 {
		return nil
	}

	// Detach from the parent context's cancellation before running anything.
	// WaitContext may reach this point precisely because the parent context
	// was canceled; handing that canceled context to the hooks would abort the
	// cleanup before it starts, and deriving a timeout from it would make the
	// deadline expire immediately and yield a false ErrShutdownTimeout.
	ctx = context.WithoutCancel(ctx)

	if timeout <= 0 {
		return runHooks(ctx, hooks)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resCh := make(chan error, 1)
	go func() { resCh <- runHooks(ctx, hooks) }()

	select {
	case err := <-resCh:
		return err
	case <-ctx.Done():
		return ErrShutdownTimeout
	}
}

// runHooks executes hooks in reverse registration order (LIFO), mirroring the
// order subsystems are usually initialized in.
//
// A failing hook does not stop the remaining ones: a subsystem that fails to
// close must not keep the others open. All errors are collected and joined,
// so errors.Is works against any of them. A single error is returned as is,
// preserving the identity the hook produced.
func runHooks(ctx context.Context, hooks []hook) error {
	errs := make([]error, 0, len(hooks))

	for i := len(hooks) - 1; i >= 0; i-- {
		if err := hooks[i].fn(ctx); err != nil {
			errs = append(errs, hooks[i].wrap(err))
		}
	}

	if len(errs) == 1 {
		return errs[0]
	}

	return errors.Join(errs...)
}

// SetLogger sets the logger used to report shutdown progress.
func (s *Shutdown) SetLogger(l Logger) *Shutdown {
	s.mu.Lock()
	s.log = l
	s.mu.Unlock()

	return s
}

// SetTimeout limits the time allowed for all cleanup hooks to complete.
// A non-positive duration (the default) means no timeout.
//
// The timeout covers the whole hook sequence, not each hook separately.
// When it elapses, WaitContext/Wait returns ErrShutdownTimeout and the context
// passed to the hooks is canceled. The hooks still run in their own goroutine,
// so a hook that ignores ctx cancellation keeps running (and leaks its
// goroutine) until it finishes on its own; long-running cleanup must honor ctx
// to be interruptible.
func (s *Shutdown) SetTimeout(d time.Duration) *Shutdown {
	s.mu.Lock()
	s.timeout = d
	s.mu.Unlock()

	return s
}

// OnDestroy registers an unnamed callback executed when the app/service is
// terminating. It appends to the set of hooks; registering a second callback
// does not replace the first.
//
// Prefer Add, which attaches a name used to attribute failures. Use
// ResetHooks to drop everything registered so far.
//
// Note for users of v2.0.x: this method used to replace the previously
// registered callback, which silently dropped cleanup registered by other
// components sharing DefaultShutdown.
func (s *Shutdown) OnDestroy(fn DestroyFunc) *Shutdown {
	return s.Add("", fn)
}

// Add registers a named cleanup hook executed when the app/service is
// terminating.
//
// Hooks run in reverse registration order (LIFO), so subsystems are torn down
// in the opposite order they were brought up. Every hook runs even if an
// earlier one failed; the errors are joined and returned by Wait/WaitContext,
// each annotated with its hook name.
//
// A nil fn is ignored.
func (s *Shutdown) Add(name string, fn DestroyFunc) *Shutdown {
	if fn == nil {
		return s
	}

	s.mu.Lock()
	s.hooks = append(s.hooks, hook{name: name, fn: fn})
	s.mu.Unlock()

	return s
}

// SetForceOnSecondSignal controls what happens when one of the watched signals
// arrives while the cleanup hooks are still running. When enabled (the
// default), the process terminates immediately with exit code 128+signum,
// giving the operator a way out of a hung cleanup — the behavior they would
// get from the OS if the process did not handle signals at all.
//
// Disable it only when the cleanup must never be interrupted; note that doing
// so leaves SIGKILL as the only way to stop a hook that ignores its context.
func (s *Shutdown) SetForceOnSecondSignal(v bool) *Shutdown {
	s.mu.Lock()
	s.force = v
	s.mu.Unlock()

	return s
}

// setExit replaces the process termination function used by the force quit.
// It is the seam that lets the tests observe a forced exit instead of dying.
func (s *Shutdown) setExit(fn func(code int)) *Shutdown {
	s.mu.Lock()
	s.exit = fn
	s.mu.Unlock()

	return s
}

// ResetHooks removes every hook registered so far. It exists for callers that
// need to replace the whole cleanup set rather than extend it.
func (s *Shutdown) ResetHooks() *Shutdown {
	s.mu.Lock()
	s.hooks = nil
	s.mu.Unlock()

	return s
}

// End triggers the shutdown manually. It is non-blocking and safe to call
// multiple times, before or after Wait: the underlying channel is closed once,
// which releases every waiter rather than just one.
func (s *Shutdown) End() {
	s.endOnce.Do(func() { close(s.done) })
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

// OnDestroy appends an unnamed destroy callback to DefaultShutdown.
func OnDestroy(fn DestroyFunc) *Shutdown {
	return DefaultShutdown.OnDestroy(fn)
}

// Add appends a named cleanup hook to DefaultShutdown.
func Add(name string, fn DestroyFunc) *Shutdown {
	return DefaultShutdown.Add(name, fn)
}

// ResetHooks removes every hook registered on DefaultShutdown.
func ResetHooks() *Shutdown {
	return DefaultShutdown.ResetHooks()
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
