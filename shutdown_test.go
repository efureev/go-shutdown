package shutdown

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// signalSelf sends the given signal to the current process after a short delay,
// giving Wait time to subscribe first.
func signalSelf(t *testing.T, s os.Signal) {
	t.Helper()

	time.Sleep(10 * time.Millisecond)

	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Errorf("find process: %v", err)
		return
	}

	if err := p.Signal(s); err != nil {
		t.Errorf("signal process: %v", err)
	}
}

func TestShutdownWaitBySignal(t *testing.T) {
	t.Run("default signals", func(t *testing.T) {
		for _, sig := range signalsDefault {
			go signalSelf(t, sig)

			if err := New().Wait(); err != nil {
				t.Fatalf("Wait(%v) returned error: %v", sig, err)
			}
		}
	})

	t.Run("exact signal", func(t *testing.T) {
		go signalSelf(t, syscall.SIGTERM)

		if err := New().Wait(syscall.SIGTERM); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
	})

	t.Run("custom signal", func(t *testing.T) {
		go signalSelf(t, syscall.SIGHUP)

		if err := New().Wait(syscall.SIGHUP); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
	})
}

func TestShutdownWaitByManualEnd(t *testing.T) {
	sh := New()

	go func() {
		time.Sleep(10 * time.Millisecond)
		sh.End()
	}()

	if err := sh.Wait(); err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
}

func TestShutdownOnDestroy(t *testing.T) {
	t.Run("without error", func(t *testing.T) {
		sh := New()
		go func() {
			time.Sleep(10 * time.Millisecond)
			sh.End()
		}()

		called := false
		err := sh.OnDestroy(func(_ context.Context) error {
			called = true
			return nil
		}).Wait()

		if err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
		if !called {
			t.Fatal("OnDestroy callback was not called")
		}
	})

	t.Run("with error", func(t *testing.T) {
		sh := New()
		go func() {
			time.Sleep(10 * time.Millisecond)
			sh.End()
		}()

		wantErr := errors.New("error test")
		err := sh.OnDestroy(func(_ context.Context) error {
			return wantErr
		}).Wait()

		if !errors.Is(err, wantErr) {
			t.Fatalf("Wait returned %v, want %v", err, wantErr)
		}
	})
}

func TestShutdownWithLogger(t *testing.T) {
	sh := New()
	go func() {
		time.Sleep(10 * time.Millisecond)
		sh.End()
	}()

	logger := new(mockLogger)
	if err := sh.SetLogger(logger).Wait(); err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}

	assertLoggerMessages(t, logger)
}

func TestDefaultShutdown(t *testing.T) {
	t.Run("manual end", func(t *testing.T) {
		useFreshDefault(t)

		go func() {
			time.Sleep(10 * time.Millisecond)
			End()
		}()

		if err := Wait(); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
	})

	t.Run("on destroy", func(t *testing.T) {
		useFreshDefault(t)

		go func() {
			time.Sleep(10 * time.Millisecond)
			End()
		}()

		called := false
		err := OnDestroy(func(_ context.Context) error {
			called = true
			return nil
		}).Wait()

		if err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
		if !called {
			t.Fatal("OnDestroy callback was not called")
		}
	})

	t.Run("with logger", func(t *testing.T) {
		useFreshDefault(t)

		go func() {
			time.Sleep(10 * time.Millisecond)
			End()
		}()

		logger := new(mockLogger)
		if err := WaitWithLogger(logger); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}

		assertLoggerMessages(t, logger)
	})

	t.Run("package-level Add and ResetHooks", func(t *testing.T) {
		useFreshDefault(t)
		ResetHooks()

		go func() {
			time.Sleep(10 * time.Millisecond)
			End()
		}()

		called := false
		Add("named", func(_ context.Context) error {
			called = true
			return nil
		})

		if err := Wait(); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
		if !called {
			t.Fatal("hook registered via package-level Add was not called")
		}

		ResetHooks()
	})
}

func TestShutdownEndSafety(t *testing.T) {
	t.Run("End before Wait does not block and stops Wait", func(t *testing.T) {
		sh := New()
		sh.End()

		done := make(chan error, 1)
		go func() { done <- sh.Wait() }()

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Wait returned error: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("Wait did not return after early End")
		}
	})

	t.Run("repeated End does not block", func(t *testing.T) {
		sh := New()

		finished := make(chan struct{})
		go func() {
			sh.End()
			sh.End()
			sh.End()
			close(finished)
		}()

		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("repeated End blocked")
		}

		if err := sh.Wait(); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
	})

	t.Run("End after Wait completed does not block", func(t *testing.T) {
		sh := New()

		go func() {
			time.Sleep(10 * time.Millisecond)
			sh.End()
		}()

		if err := sh.Wait(); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}

		finished := make(chan struct{})
		go func() {
			sh.End()
			close(finished)
		}()

		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("End after Wait blocked")
		}
	})
}

func TestShutdownWaitContext(t *testing.T) {
	t.Run("returns when context is canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())

		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		if err := New().WaitContext(ctx); err != nil {
			t.Fatalf("WaitContext returned error: %v", err)
		}
	})

	t.Run("context cancellation with timeout still runs destroy", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())

		ran := make(chan struct{}, 1)
		sh := New().
			SetTimeout(time.Second).
			OnDestroy(func(_ context.Context) error {
				ran <- struct{}{}
				return nil
			})

		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		if err := sh.WaitContext(ctx); err != nil {
			t.Fatalf("WaitContext returned %v, want nil", err)
		}

		select {
		case <-ran:
		case <-time.After(time.Second):
			t.Fatal("destroy callback did not run after context cancellation")
		}
	})

	t.Run("OnDestroy context is canceled on timeout", func(t *testing.T) {
		canceled := make(chan struct{}, 1)

		sh := New().
			SetTimeout(20 * time.Millisecond).
			OnDestroy(func(ctx context.Context) error {
				<-ctx.Done()
				canceled <- struct{}{}
				return ctx.Err()
			})

		go func() {
			time.Sleep(10 * time.Millisecond)
			sh.End()
		}()

		if err := sh.Wait(); !errors.Is(err, ErrShutdownTimeout) {
			t.Fatalf("Wait returned %v, want %v", err, ErrShutdownTimeout)
		}

		select {
		case <-canceled:
		case <-time.After(time.Second):
			t.Fatal("destroy context was not canceled on timeout")
		}
	})
}

func TestShutdownTimeout(t *testing.T) {
	t.Run("slow destroy returns ErrShutdownTimeout", func(t *testing.T) {
		sh := New().
			SetTimeout(20 * time.Millisecond).
			OnDestroy(func(_ context.Context) error {
				time.Sleep(500 * time.Millisecond)
				return nil
			})

		go func() {
			time.Sleep(10 * time.Millisecond)
			sh.End()
		}()

		if err := sh.Wait(); !errors.Is(err, ErrShutdownTimeout) {
			t.Fatalf("Wait returned %v, want %v", err, ErrShutdownTimeout)
		}
	})

	t.Run("fast destroy completes within timeout", func(t *testing.T) {
		sh := New().
			SetTimeout(time.Second).
			OnDestroy(func(_ context.Context) error { return nil })

		go func() {
			time.Sleep(10 * time.Millisecond)
			sh.End()
		}()

		if err := sh.Wait(); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
	})
}

func TestShutdownHooks(t *testing.T) {
	t.Run("OnDestroy accumulates instead of replacing", func(t *testing.T) {
		sh := New()
		go endAfterDelay(sh)

		var called []string
		sh.OnDestroy(func(_ context.Context) error {
			called = append(called, "first")
			return nil
		})
		sh.OnDestroy(func(_ context.Context) error {
			called = append(called, "second")
			return nil
		})

		if err := sh.Wait(); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
		if len(called) != 2 {
			t.Fatalf("ran %d hooks (%v), want 2", len(called), called)
		}
	})

	t.Run("hooks run in reverse registration order", func(t *testing.T) {
		sh := New()
		go endAfterDelay(sh)

		var order []string
		for _, name := range []string{"first", "second", "third"} {
			sh.Add(name, func(_ context.Context) error {
				order = append(order, name)
				return nil
			})
		}

		if err := sh.Wait(); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}

		want := []string{"third", "second", "first"}
		if !slices.Equal(order, want) {
			t.Fatalf("order = %v, want %v", order, want)
		}
	})

	t.Run("a failing hook does not stop the others", func(t *testing.T) {
		sh := New()
		go endAfterDelay(sh)

		errFirst := errors.New("first failed")
		errSecond := errors.New("second failed")

		ran := 0
		sh.Add("first", func(_ context.Context) error { ran++; return errFirst })
		sh.Add("second", func(_ context.Context) error { ran++; return errSecond })
		sh.Add("third", func(_ context.Context) error { ran++; return nil })

		err := sh.Wait()

		if ran != 3 {
			t.Fatalf("ran %d hooks, want 3", ran)
		}
		if !errors.Is(err, errFirst) || !errors.Is(err, errSecond) {
			t.Fatalf("err = %v, want it to wrap both %v and %v", err, errFirst, errSecond)
		}
		if !strings.Contains(err.Error(), `"first"`) || !strings.Contains(err.Error(), `"second"`) {
			t.Errorf("err = %q, want it to name the failing hooks", err)
		}
	})

	t.Run("single unnamed hook keeps its error identity", func(t *testing.T) {
		sh := New()
		go endAfterDelay(sh)

		wantErr := errors.New("boom")
		err := sh.OnDestroy(func(_ context.Context) error { return wantErr }).Wait()

		if err != wantErr { //nolint:errorlint // identity is the property under test
			t.Fatalf("err = %v, want the exact error value %v", err, wantErr)
		}
	})

	t.Run("ResetHooks drops everything registered so far", func(t *testing.T) {
		sh := New()
		go endAfterDelay(sh)

		called := false
		sh.Add("dropped", func(_ context.Context) error { called = true; return nil })
		sh.ResetHooks()

		if err := sh.Wait(); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
		if called {
			t.Fatal("hook ran after ResetHooks")
		}
	})

	t.Run("nil hook is ignored", func(t *testing.T) {
		sh := New()
		go endAfterDelay(sh)

		if err := sh.Add("nil", nil).Wait(); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
	})
}

// TestShutdownDestroyContextDetached pins the fix for the case where the
// cleanup was handed the very context whose cancellation started it.
func TestShutdownDestroyContextDetached(t *testing.T) {
	run := func(t *testing.T, timeout time.Duration) {
		t.Helper()

		ctx, cancel := context.WithCancel(t.Context())

		got := make(chan error, 1)
		sh := New().
			SetTimeout(timeout).
			OnDestroy(func(c context.Context) error {
				got <- c.Err()
				return nil
			})

		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		if err := sh.WaitContext(ctx); err != nil {
			t.Fatalf("WaitContext returned error: %v", err)
		}

		select {
		case err := <-got:
			if err != nil {
				t.Fatalf("hook context was already canceled (%v); cleanup cannot run", err)
			}
		case <-time.After(time.Second):
			t.Fatal("hook did not run after context cancellation")
		}
	}

	t.Run("without timeout", func(t *testing.T) { run(t, 0) })
	t.Run("with timeout", func(t *testing.T) { run(t, time.Second) })
}

// TestShutdownWaitIsIdempotent pins the fix for a repeated Wait hanging
// forever on an instance whose cleanup had already finished.
func TestShutdownWaitIsIdempotent(t *testing.T) {
	t.Run("a later Wait returns the same error immediately", func(t *testing.T) {
		wantErr := errors.New("cleanup failed")

		runs := 0
		sh := New().OnDestroy(func(_ context.Context) error {
			runs++
			return wantErr
		})

		go endAfterDelay(sh)

		if err := sh.Wait(); !errors.Is(err, wantErr) {
			t.Fatalf("first Wait returned %v, want %v", err, wantErr)
		}

		second := make(chan error, 1)
		go func() { second <- sh.Wait() }()

		select {
		case err := <-second:
			if !errors.Is(err, wantErr) {
				t.Fatalf("second Wait returned %v, want %v", err, wantErr)
			}
		case <-time.After(time.Second):
			t.Fatal("second Wait blocked instead of returning the stored result")
		}

		if runs != 1 {
			t.Fatalf("hooks ran %d times, want exactly 1", runs)
		}
	})

	t.Run("concurrent Wait callers share a single cleanup run", func(t *testing.T) {
		var runs atomic.Int32

		sh := New().OnDestroy(func(_ context.Context) error {
			runs.Add(1)
			return nil
		})

		const waiters = 4

		errs := make(chan error, waiters)
		for range waiters {
			go func() { errs <- sh.Wait() }()
		}

		time.Sleep(20 * time.Millisecond)
		sh.End()

		for range waiters {
			select {
			case err := <-errs:
				if err != nil {
					t.Fatalf("Wait returned error: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("a concurrent Wait did not return")
			}
		}

		if got := runs.Load(); got != 1 {
			t.Fatalf("hooks ran %d times, want exactly 1", got)
		}
	})
}

// TestShutdownForceOnSecondSignal pins the fix for a signal arriving during
// cleanup being swallowed, which left a hung hook uninterruptible.
func TestShutdownForceOnSecondSignal(t *testing.T) {
	t.Run("enabled by default: the second signal terminates the process", func(t *testing.T) {
		forced := make(chan int, 1)

		sh := New()
		sh.setExit(func(code int) { forced <- code })

		sh.OnDestroy(func(_ context.Context) error {
			go signalSelf(t, syscall.SIGTERM) // the second signal

			select {
			case code := <-forced:
				if want := 128 + int(syscall.SIGTERM); code != want {
					t.Errorf("exit code = %d, want %d", code, want)
				}
			case <-time.After(2 * time.Second):
				t.Error("second signal did not force a quit; cleanup is uninterruptible")
			}

			return nil
		})

		go signalSelf(t, syscall.SIGTERM) // the first signal

		if err := sh.Wait(syscall.SIGTERM); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
	})

	t.Run("disabled: the second signal is ignored", func(t *testing.T) {
		sh := New().SetForceOnSecondSignal(false)
		sh.setExit(func(int) { t.Error("process terminated while the force quit was disabled") })

		sh.OnDestroy(func(_ context.Context) error {
			go signalSelf(t, syscall.SIGTERM)
			time.Sleep(200 * time.Millisecond)

			return nil
		})

		go signalSelf(t, syscall.SIGTERM)

		if err := sh.Wait(syscall.SIGTERM); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
	})
}

// TestShutdownReason pins the fix for the caller being unable to tell what
// triggered the shutdown, or which signal it was.
func TestShutdownReason(t *testing.T) {
	t.Run("before the shutdown is triggered", func(t *testing.T) {
		sh := New()

		if got := sh.Reason(); got != ReasonNone {
			t.Errorf("Reason() = %v, want %v", got, ReasonNone)
		}
		if got := sh.Signal(); got != nil {
			t.Errorf("Signal() = %v, want nil", got)
		}
		if got := sh.ExitCode(); got != 0 {
			t.Errorf("ExitCode() = %d, want 0", got)
		}
	})

	t.Run("triggered by a signal", func(t *testing.T) {
		sh := New()
		go signalSelf(t, syscall.SIGTERM)

		if err := sh.Wait(syscall.SIGTERM); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}

		if got := sh.Reason(); got != ReasonSignal {
			t.Errorf("Reason() = %v, want %v", got, ReasonSignal)
		}
		if got := sh.Signal(); got != syscall.SIGTERM {
			t.Errorf("Signal() = %v, want %v", got, syscall.SIGTERM)
		}
		if want := 128 + int(syscall.SIGTERM); sh.ExitCode() != want {
			t.Errorf("ExitCode() = %d, want %d", sh.ExitCode(), want)
		}
	})

	t.Run("triggered by a canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		sh := New()
		if err := sh.WaitContext(ctx); err != nil {
			t.Fatalf("WaitContext returned error: %v", err)
		}

		if got := sh.Reason(); got != ReasonContext {
			t.Errorf("Reason() = %v, want %v", got, ReasonContext)
		}
		if got := sh.Signal(); got != nil {
			t.Errorf("Signal() = %v, want nil", got)
		}
		if got := sh.ExitCode(); got != 0 {
			t.Errorf("ExitCode() = %d, want 0", got)
		}
	})

	t.Run("triggered by End", func(t *testing.T) {
		sh := New()
		go endAfterDelay(sh)

		if err := sh.Wait(); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}

		if got := sh.Reason(); got != ReasonManual {
			t.Errorf("Reason() = %v, want %v", got, ReasonManual)
		}
		if got := sh.ExitCode(); got != 0 {
			t.Errorf("ExitCode() = %d, want 0", got)
		}
	})

	t.Run("a later trigger does not relabel the reason", func(t *testing.T) {
		sh := New()
		go signalSelf(t, syscall.SIGTERM)

		if err := sh.Wait(syscall.SIGTERM); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}

		// Wakes on the closed done channel and would report ReasonManual.
		sh.End()
		if err := sh.Wait(syscall.SIGTERM); err != nil {
			t.Fatalf("second Wait returned error: %v", err)
		}

		if got := sh.Reason(); got != ReasonSignal {
			t.Errorf("Reason() = %v, want %v", got, ReasonSignal)
		}
		if got := sh.Signal(); got != syscall.SIGTERM {
			t.Errorf("Signal() = %v, want %v", got, syscall.SIGTERM)
		}
	})
}

func TestReasonString(t *testing.T) {
	tests := map[Reason]string{
		ReasonNone:    "none",
		ReasonSignal:  "signal",
		ReasonContext: "context",
		ReasonManual:  "manual",
		Reason(42):    "unknown",
	}

	for r, want := range tests {
		if got := r.String(); got != want {
			t.Errorf("Reason(%d).String() = %q, want %q", int(r), got, want)
		}
	}
}

func TestExitCode(t *testing.T) {
	tests := map[string]struct {
		sig  os.Signal
		want int
	}{
		"SIGINT":     {syscall.SIGINT, 130},
		"SIGTERM":    {syscall.SIGTERM, 143},
		"SIGQUIT":    {syscall.SIGQUIT, 131},
		"non-signal": {fakeSignal{}, 1},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := exitCode(tc.sig); got != tc.want {
				t.Errorf("exitCode(%v) = %d, want %d", tc.sig, got, tc.want)
			}
		})
	}
}

type fakeSignal struct{}

func (fakeSignal) String() string { return "fake" }
func (fakeSignal) Signal()        {}

// endAfterDelay triggers a shutdown once Wait has had time to subscribe.
func endAfterDelay(sh *Shutdown) {
	time.Sleep(10 * time.Millisecond)
	sh.End()
}

// useFreshDefault swaps DefaultShutdown for an unused instance and leaves
// another one behind. A Shutdown runs its hooks once, so tests driving a full
// shutdown cycle cannot share the global instance.
func useFreshDefault(t *testing.T) {
	t.Helper()

	DefaultShutdown = New()
	t.Cleanup(func() { DefaultShutdown = New() })
}

func assertLoggerMessages(t *testing.T, logger *mockLogger) {
	t.Helper()

	if len(logger.Logs) < 2 {
		t.Fatalf("expected at least 2 log messages, got %d", len(logger.Logs))
	}
	if logger.Logs[0] != `shutdown started...` {
		t.Errorf("Logs[0] = %v, want %q", logger.Logs[0], `shutdown started...`)
	}
	if logger.Logs[1] != `shutdown complete...` {
		t.Errorf("Logs[1] = %v, want %q", logger.Logs[1], `shutdown complete...`)
	}
}

type mockLogger struct {
	Logs []any
}

func (l *mockLogger) Info(args ...any) {
	l.Logs = append(l.Logs, args...)
}

func (l *mockLogger) Trace(args ...any) {
	l.Info(args...)
}
