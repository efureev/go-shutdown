package shutdown

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// HookFunc is a cleanup callback executed during shutdown.
//
// The provided context is canceled when a timeout elapses (see WithTimeout and
// HookTimeout). It is deliberately detached from the context passed to Wait:
// canceling that context is one of the ways to trigger the shutdown, so it
// must not abort the cleanup it just triggered.
type HookFunc func(ctx context.Context) error

// HookOption configures a single hook at registration.
type HookOption func(*hook)

// Parallel marks a hook as safe to run concurrently with its neighbors.
//
// Consecutive parallel hooks form one group and run at the same time; a hook
// without this option acts as a barrier between groups. Registration order
// therefore still decides the teardown order:
//
//	sh.Add("db", closeDB).
//		Add("cache", closeCache, shutdown.Parallel()).
//		Add("search", closeSearch, shutdown.Parallel()).
//		Add("http", stopServer)
//
//	// teardown: http, then cache and search together, then db
func Parallel() HookOption {
	return func(h *hook) { h.parallel = true }
}

// HookTimeout limits this hook alone. It is independent of WithTimeout, which
// bounds the whole sequence; whichever expires first wins.
//
// On expiry the hook's context is canceled and Wait reports a HookError
// wrapping ErrTimeout. The hook keeps running in its own goroutine until it
// returns, so it must honor ctx to be interruptible.
func HookTimeout(d time.Duration) HookOption {
	return func(h *hook) { h.timeout = d }
}

// hook is a cleanup callback together with how it should be run.
type hook struct {
	name     string
	fn       HookFunc
	parallel bool
	timeout  time.Duration
}

// Add registers a named cleanup hook executed when the app/service terminates.
//
// Hooks run in reverse registration order (LIFO), so subsystems are torn down
// in the opposite order they were brought up. Every hook runs even if an
// earlier one failed: a subsystem that fails to close must not keep the others
// open. Failures are joined into the error returned by Wait, each wrapped in a
// HookError carrying the name.
//
// A nil fn is ignored.
func (s *Shutdown) Add(name string, fn HookFunc, opts ...HookOption) *Shutdown {
	if fn == nil {
		return s
	}

	h := hook{name: name, fn: fn}
	for _, opt := range opts {
		opt(&h)
	}

	s.mu.Lock()
	s.hooks = append(s.hooks, h)
	s.mu.Unlock()

	return s
}

// runState collects hook results by index, so the joined error keeps a
// deterministic order even when a group finished out of order.
type runState struct {
	errs     []error
	finished []atomic.Bool
}

// runHooks executes every registered hook under the global timeout, if any.
func (s *Shutdown) runHooks(ctx context.Context) error {
	s.mu.Lock()
	hooks := slices.Clone(s.hooks)
	s.mu.Unlock()

	if len(hooks) == 0 {
		return nil
	}

	// Detach from the parent context's cancellation before running anything.
	// Wait may reach this point precisely because the parent context was
	// canceled; handing that canceled context to the hooks would abort the
	// cleanup before it starts, and deriving a timeout from it would make the
	// deadline expire immediately and yield a false ErrTimeout.
	ctx = context.WithoutCancel(ctx)

	st := &runState{
		errs:     make([]error, len(hooks)),
		finished: make([]atomic.Bool, len(hooks)),
	}

	if s.timeout <= 0 {
		return s.execute(ctx, hooks, st)
	}

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	res := make(chan error, 1)
	go func() { res <- s.execute(ctx, hooks, st) }()

	select {
	case err := <-res:
		return err
	case <-ctx.Done():
		// The abandoned goroutine keeps writing to st.errs; only the atomic
		// finished flags may be read from here.
		return timeoutError(hooks, st, s.timeout)
	}
}

// execute walks the hooks in reverse registration order, running each maximal
// run of consecutive parallel hooks as one concurrent group.
func (s *Shutdown) execute(ctx context.Context, hooks []hook, st *runState) error {
	for i := len(hooks) - 1; i >= 0; {
		if !hooks[i].parallel {
			s.runOne(ctx, hooks[i], i, st)
			i--

			continue
		}

		first := i
		for first >= 0 && hooks[first].parallel {
			first--
		}

		var wg sync.WaitGroup
		for k := first + 1; k <= i; k++ {
			wg.Add(1)

			go func() {
				defer wg.Done()
				s.runOne(ctx, hooks[k], k, st)
			}()
		}
		wg.Wait()

		i = first
	}

	return st.join()
}

// runOne executes a single hook and records its outcome.
func (s *Shutdown) runOne(ctx context.Context, h hook, i int, st *runState) {
	start := time.Now()
	err := runHook(ctx, h)
	st.finished[i].Store(true)

	if err != nil {
		st.errs[i] = &HookError{Hook: h.name, Err: err}
		s.log.Error("hook failed", "hook", h.name, "duration", time.Since(start), "error", err)

		return
	}

	s.log.Debug("hook finished", "hook", h.name, "duration", time.Since(start))
}

// runHook applies the per-hook timeout, if configured.
func runHook(ctx context.Context, h hook) error {
	if h.timeout <= 0 {
		return h.fn(ctx)
	}

	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	res := make(chan error, 1)
	go func() { res <- h.fn(ctx) }()

	select {
	case err := <-res:
		return err
	case <-ctx.Done():
		return ErrTimeout
	}
}

// join collects the recorded errors in execution order.
func (st *runState) join() error {
	errs := make([]error, 0, len(st.errs))

	for i := len(st.errs) - 1; i >= 0; i-- {
		if st.errs[i] != nil {
			errs = append(errs, st.errs[i])
		}
	}

	if len(errs) == 1 {
		return errs[0]
	}

	return errors.Join(errs...)
}

// timeoutError names the hooks that were still running when the budget ran
// out. Without the names the caller cannot tell which subsystem hung.
func timeoutError(hooks []hook, st *runState, d time.Duration) error {
	var stuck []string

	for i := range hooks {
		if !st.finished[i].Load() {
			stuck = append(stuck, hooks[i].name)
		}
	}

	if len(stuck) == 0 {
		return ErrTimeout
	}

	return fmt.Errorf("%w after %s, still running: %s", ErrTimeout, d, strings.Join(stuck, ", "))
}
