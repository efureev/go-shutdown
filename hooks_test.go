package shutdown

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recorder collects hook names in execution order.
type recorder struct {
	mu    sync.Mutex
	order []string
}

func (r *recorder) hook(name string) HookFunc {
	return func(context.Context) error {
		r.mu.Lock()
		r.order = append(r.order, name)
		r.mu.Unlock()

		return nil
	}
}

func (r *recorder) result() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string(nil), r.order...)
}

// runNow triggers a shutdown and returns the cleanup error.
func runNow(t *testing.T, sh *Shutdown) error {
	t.Helper()

	sh.End()

	return sh.Wait(context.Background())
}

func TestHooksOrder(t *testing.T) {
	t.Parallel()

	t.Run("reverse registration order", func(t *testing.T) {
		t.Parallel()

		rec := &recorder{}
		sh := New(newFakeSignals().option()).
			Add("first", rec.hook("first")).
			Add("second", rec.hook("second")).
			Add("third", rec.hook("third"))

		if err := runNow(t, sh); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}

		want := []string{"third", "second", "first"}
		if got := rec.result(); !equal(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})

	t.Run("nil hook is ignored", func(t *testing.T) {
		t.Parallel()

		sh := New(newFakeSignals().option()).Add("nil", nil)

		if err := runNow(t, sh); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
	})
}

func TestHooksFailure(t *testing.T) {
	t.Parallel()

	t.Run("a failing hook does not stop the others", func(t *testing.T) {
		t.Parallel()

		errFirst := errors.New("first failed")
		errSecond := errors.New("second failed")

		ran := 0
		sh := New(newFakeSignals().option()).
			Add("first", func(context.Context) error { ran++; return errFirst }).
			Add("second", func(context.Context) error { ran++; return errSecond }).
			Add("third", func(context.Context) error { ran++; return nil })

		err := runNow(t, sh)

		if ran != 3 {
			t.Fatalf("ran %d hooks, want 3", ran)
		}
		if !errors.Is(err, errFirst) || !errors.Is(err, errSecond) {
			t.Fatalf("err = %v, want it to wrap both %v and %v", err, errFirst, errSecond)
		}
	})

	t.Run("errors.As reaches the failing hook", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("boom")
		sh := New(newFakeSignals().option()).
			Add("db", func(context.Context) error { return wantErr })

		err := runNow(t, sh)

		var hookErr *HookError
		if !errors.As(err, &hookErr) {
			t.Fatalf("err = %v, want a *HookError", err)
		}
		if hookErr.Hook != "db" {
			t.Errorf("HookError.Hook = %q, want %q", hookErr.Hook, "db")
		}
		if !errors.Is(hookErr, wantErr) {
			t.Errorf("HookError does not unwrap to %v", wantErr)
		}
	})

	t.Run("error order is deterministic across parallel hooks", func(t *testing.T) {
		t.Parallel()

		// The last hook is released first, so completion order is the reverse
		// of the reported order unless the errors are indexed.
		gate := make(chan struct{})

		sh := New(newFakeSignals().option()).
			Add("a", func(context.Context) error {
				<-gate

				return errors.New("a failed")
			}, Parallel()).
			Add("b", func(context.Context) error {
				close(gate)

				return errors.New("b failed")
			}, Parallel())

		err := runNow(t, sh)

		// Execution order is LIFO: b, then a.
		want := `shutdown: hook "b": b failed` + "\n" + `shutdown: hook "a": a failed`
		if err == nil || err.Error() != want {
			t.Fatalf("err = %v, want:\n%s", err, want)
		}
	})
}

func TestHooksParallel(t *testing.T) {
	t.Parallel()

	t.Run("a parallel group runs concurrently", func(t *testing.T) {
		t.Parallel()

		const n = 3

		// Barrier rather than timing: every hook of the group must be running
		// before any of them may finish. Run sequentially, the first hook
		// blocks until the shutdown budget expires and Wait reports an error.
		var wg sync.WaitGroup
		wg.Add(n)

		released := make(chan struct{})
		go func() { wg.Wait(); close(released) }()

		sh := New(newFakeSignals().option(), WithTimeout(2*time.Second))
		for i := range n {
			sh.Add(fmt.Sprintf("h%d", i), func(ctx context.Context) error {
				wg.Done()

				select {
				case <-released:
					return nil
				case <-ctx.Done():
					return errors.New("hooks did not run concurrently")
				}
			}, Parallel())
		}

		if err := runNow(t, sh); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
	})

	t.Run("a sequential hook is a barrier between groups", func(t *testing.T) {
		t.Parallel()

		rec := &recorder{}
		sh := New(newFakeSignals().option()).
			Add("a", rec.hook("a"), Parallel()).
			Add("b", rec.hook("b"), Parallel()).
			Add("c", rec.hook("c")).
			Add("d", rec.hook("d"), Parallel()).
			Add("e", rec.hook("e"), Parallel())

		if err := runNow(t, sh); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}

		got := rec.result()
		if len(got) != 5 {
			t.Fatalf("ran %d hooks (%v), want 5", len(got), got)
		}

		// Execution: {e,d} in any order, then c, then {b,a} in any order.
		if !sameSet(got[:2], []string{"d", "e"}) {
			t.Errorf("first group = %v, want d and e", got[:2])
		}
		if got[2] != "c" {
			t.Errorf("barrier = %q, want %q", got[2], "c")
		}
		if !sameSet(got[3:], []string{"a", "b"}) {
			t.Errorf("second group = %v, want a and b", got[3:])
		}
	})
}

func TestHookTimeout(t *testing.T) {
	t.Parallel()

	t.Run("a stuck hook times out without blocking its neighbors", func(t *testing.T) {
		t.Parallel()

		release := make(chan struct{})
		t.Cleanup(func() { close(release) })

		ran := make(chan string, 2)

		sh := New(newFakeSignals().option()).
			Add("fast", func(context.Context) error {
				ran <- "fast"

				return nil
			}).
			Add("stuck", func(ctx context.Context) error {
				ran <- "stuck"
				select {
				case <-release:
				case <-ctx.Done():
				}

				return nil
			}, HookTimeout(50*time.Millisecond))

		err := runNow(t, sh)

		var hookErr *HookError
		if !errors.As(err, &hookErr) {
			t.Fatalf("err = %v, want a *HookError", err)
		}
		if hookErr.Hook != "stuck" {
			t.Errorf("HookError.Hook = %q, want %q", hookErr.Hook, "stuck")
		}
		if !errors.Is(err, ErrTimeout) {
			t.Errorf("err = %v, want it to wrap ErrTimeout", err)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v, want it to wrap context.DeadlineExceeded", err)
		}

		if len(ran) != 2 {
			t.Errorf("ran %d hooks, want both", len(ran))
		}
	})

	t.Run("combines with Parallel", func(t *testing.T) {
		t.Parallel()

		sh := New(newFakeSignals().option()).
			Add("quick", func(context.Context) error { return nil },
				Parallel(), HookTimeout(time.Second))

		if err := runNow(t, sh); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
	})
}

func TestGlobalTimeout(t *testing.T) {
	t.Parallel()

	t.Run("names the hooks that were still running", func(t *testing.T) {
		t.Parallel()

		release := make(chan struct{})
		t.Cleanup(func() { close(release) })

		// db deliberately ignores its context — that is the case where naming
		// it matters, since a hook that honors ctx returns on its own.
		sh := New(newFakeSignals().option(), WithTimeout(50*time.Millisecond)).
			Add("db", func(context.Context) error {
				<-release

				return nil
			}).
			Add("http", func(context.Context) error { return nil })

		err := runNow(t, sh)

		if !errors.Is(err, ErrTimeout) {
			t.Fatalf("err = %v, want it to wrap ErrTimeout", err)
		}
		if !strings.Contains(err.Error(), "db") {
			t.Errorf("err = %q, want it to name the stuck hook", err)
		}
		if strings.Contains(err.Error(), "http") {
			t.Errorf("err = %q, named a hook that had finished", err)
		}
	})

	t.Run("a fast sequence finishes within the budget", func(t *testing.T) {
		t.Parallel()

		sh := New(newFakeSignals().option(), WithTimeout(2*time.Second)).
			Add("quick", func(context.Context) error { return nil })

		if err := runNow(t, sh); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
	})
}

func TestTimeoutError(t *testing.T) {
	t.Parallel()

	hooks := []hook{{name: "a"}, {name: "b"}}

	t.Run("names only the unfinished hooks", func(t *testing.T) {
		t.Parallel()

		st := &runState{errs: make([]error, 2), finished: make([]atomic.Bool, 2)}
		st.finished[1].Store(true)

		err := timeoutError(hooks, st, time.Second)

		if !strings.Contains(err.Error(), "a") || strings.Contains(err.Error(), "b") {
			t.Errorf("err = %q, want it to name only %q", err, "a")
		}
	})

	t.Run("plain ErrTimeout when everything finished in the race", func(t *testing.T) {
		t.Parallel()

		st := &runState{errs: make([]error, 2), finished: make([]atomic.Bool, 2)}
		st.finished[0].Store(true)
		st.finished[1].Store(true)

		if err := timeoutError(hooks, st, time.Second); !errors.Is(err, ErrTimeout) {
			t.Errorf("err = %v, want ErrTimeout", err)
		}
	})
}

// TestHookContextDetached pins the rule that cleanup is not aborted by the
// very cancellation that started it.
func TestHookContextDetached(t *testing.T) {
	t.Parallel()

	for _, timeout := range []time.Duration{0, time.Second} {
		t.Run(fmt.Sprintf("timeout=%v", timeout), func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())

			got := make(chan error, 1)
			sh := New(newFakeSignals().option(), WithTimeout(timeout)).
				Add("probe", func(c context.Context) error {
					got <- c.Err()

					return nil
				})

			res := make(chan error, 1)
			go func() { res <- sh.Wait(ctx) }()

			cancel()

			if err := waitErr(t, res); err != nil {
				t.Fatalf("Wait returned error: %v", err)
			}

			if err := <-got; err != nil {
				t.Fatalf("hook context was already canceled (%v); cleanup cannot run", err)
			}
		})
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}

	seen := make(map[string]bool, len(got))
	for _, v := range got {
		seen[v] = true
	}

	for _, v := range want {
		if !seen[v] {
			return false
		}
	}

	return true
}
