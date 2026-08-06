package shutdown

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestWaitTriggers(t *testing.T) {
	t.Parallel()

	t.Run("signal", func(t *testing.T) {
		t.Parallel()

		fs := newFakeSignals()
		sh := New(fs.option())

		res := make(chan error, 1)
		go func() { res <- sh.Wait(context.Background()) }()

		fs.send(t, syscall.SIGTERM)

		if err := waitErr(t, res); err != nil {
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

	t.Run("canceled context", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		fs := newFakeSignals()
		sh := New(fs.option())

		res := make(chan error, 1)
		go func() { res <- sh.Wait(ctx) }()

		cancel()

		if err := waitErr(t, res); err != nil {
			t.Fatalf("Wait returned error: %v", err)
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

	t.Run("manual End", func(t *testing.T) {
		t.Parallel()

		fs := newFakeSignals()
		sh := New(fs.option())

		res := make(chan error, 1)
		go func() { res <- sh.Wait(context.Background()) }()

		sh.End()

		if err := waitErr(t, res); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}

		if got := sh.Reason(); got != ReasonManual {
			t.Errorf("Reason() = %v, want %v", got, ReasonManual)
		}
		if got := sh.ExitCode(); got != 0 {
			t.Errorf("ExitCode() = %d, want 0", got)
		}
	})

	t.Run("End before Wait returns immediately", func(t *testing.T) {
		t.Parallel()

		sh := New(newFakeSignals().option())
		sh.End()

		res := make(chan error, 1)
		go func() { res <- sh.Wait(context.Background()) }()

		if err := waitErr(t, res); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
	})

	t.Run("a later trigger does not relabel the reason", func(t *testing.T) {
		t.Parallel()

		fs := newFakeSignals()
		sh := New(fs.option())

		res := make(chan error, 1)
		go func() { res <- sh.Wait(context.Background()) }()

		fs.send(t, syscall.SIGTERM)
		waitErr(t, res)

		sh.End()

		if err := sh.Wait(context.Background()); err != nil {
			t.Fatalf("second Wait returned error: %v", err)
		}

		if got := sh.Reason(); got != ReasonSignal {
			t.Errorf("Reason() = %v, want %v", got, ReasonSignal)
		}
	})
}

func TestWaitIsIdempotent(t *testing.T) {
	t.Parallel()

	t.Run("a later Wait returns the same error immediately", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("cleanup failed")

		var runs atomic.Int32

		sh := New(newFakeSignals().option())
		sh.Add("failing", func(context.Context) error {
			runs.Add(1)

			return wantErr
		})
		sh.End()

		first := sh.Wait(context.Background())
		if !errors.Is(first, wantErr) {
			t.Fatalf("first Wait returned %v, want it to wrap %v", first, wantErr)
		}

		res := make(chan error, 1)
		go func() { res <- sh.Wait(context.Background()) }()

		if second := waitErr(t, res); !errors.Is(second, wantErr) {
			t.Fatalf("second Wait returned %v, want it to wrap %v", second, wantErr)
		}

		if got := runs.Load(); got != 1 {
			t.Fatalf("hooks ran %d times, want exactly 1", got)
		}
	})

	t.Run("concurrent Wait callers share a single cleanup run", func(t *testing.T) {
		t.Parallel()

		var runs atomic.Int32

		sh := New(newFakeSignals().option())
		sh.Add("counted", func(context.Context) error {
			runs.Add(1)

			return nil
		})

		const waiters = 4

		res := make(chan error, waiters)
		for range waiters {
			go func() { res <- sh.Wait(context.Background()) }()
		}

		sh.End()

		for range waiters {
			if err := waitErr(t, res); err != nil {
				t.Fatalf("Wait returned error: %v", err)
			}
		}

		if got := runs.Load(); got != 1 {
			t.Fatalf("hooks ran %d times, want exactly 1", got)
		}
	})
}

func TestDoneAndContext(t *testing.T) {
	t.Parallel()

	t.Run("open until the shutdown starts", func(t *testing.T) {
		t.Parallel()

		sh := New(newFakeSignals().option())

		select {
		case <-sh.Done():
			t.Fatal("Done() was closed before the shutdown started")
		default:
		}

		if err := sh.Context().Err(); err != nil {
			t.Fatalf("Context() was canceled before the shutdown started: %v", err)
		}
	})

	t.Run("released when the shutdown starts", func(t *testing.T) {
		t.Parallel()

		fs := newFakeSignals()
		sh := New(fs.option())

		// A worker that reacts without ever calling Wait.
		observed := make(chan struct{})
		go func() {
			<-sh.Done()
			close(observed)
		}()

		res := make(chan error, 1)
		go func() { res <- sh.Wait(context.Background()) }()

		fs.send(t, syscall.SIGINT)
		waitErr(t, res)

		select {
		case <-observed:
		case <-time.After(2 * time.Second):
			t.Fatal("Done() was not released for a worker outside Wait")
		}

		if err := sh.Context().Err(); !errors.Is(err, context.Canceled) {
			t.Errorf("Context().Err() = %v, want %v", err, context.Canceled)
		}
		if cause := context.Cause(sh.Context()); !errors.Is(cause, ErrShutdown) {
			t.Errorf("context.Cause() = %v, want it to wrap %v", cause, ErrShutdown)
		}
	})
}

func TestForceOnSecondSignal(t *testing.T) {
	t.Parallel()

	t.Run("enabled by default: the second signal terminates the process", func(t *testing.T) {
		t.Parallel()

		forced := make(chan int, 1)
		fs := newFakeSignals()

		sh := New(fs.option(), func(s *Shutdown) {
			s.exit = func(code int) { forced <- code }
		})

		hookEntered := make(chan struct{})
		sh.Add("slow", func(context.Context) error {
			close(hookEntered)

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

		res := make(chan error, 1)
		go func() { res <- sh.Wait(context.Background()) }()

		fs.send(t, syscall.SIGTERM)
		<-hookEntered
		fs.send(t, syscall.SIGTERM)

		if err := waitErr(t, res); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
	})

	t.Run("disabled: the second signal is ignored", func(t *testing.T) {
		t.Parallel()

		fs := newFakeSignals()

		sh := New(fs.option(), WithForceOnSecondSignal(false), func(s *Shutdown) {
			s.exit = func(int) { t.Error("process terminated while the force quit was disabled") }
		})

		hookEntered := make(chan struct{})
		sh.Add("slow", func(context.Context) error {
			close(hookEntered)
			time.Sleep(100 * time.Millisecond)

			return nil
		})

		res := make(chan error, 1)
		go func() { res <- sh.Wait(context.Background()) }()

		fs.send(t, syscall.SIGTERM)
		<-hookEntered
		fs.send(t, syscall.SIGTERM)

		if err := waitErr(t, res); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
	})
}

// TestWaitByRealSignal is the one integration test kept on the real signal
// path: everything else runs through the fakeSignals seam. It must stay
// serial, because a signal is delivered to the process, not to a test.
func TestWaitByRealSignal(t *testing.T) {
	for _, sig := range signalsDefault {
		sh := New()

		res := make(chan error, 1)
		go func() { res <- sh.Wait(context.Background()) }()

		// Give Wait time to call signal.Notify; without it the default
		// handler kills the whole test binary.
		time.Sleep(50 * time.Millisecond)

		p, err := os.FindProcess(os.Getpid())
		if err != nil {
			t.Fatalf("find process: %v", err)
		}
		if err := p.Signal(sig); err != nil {
			t.Fatalf("signal process: %v", err)
		}

		if err := waitErr(t, res); err != nil {
			t.Fatalf("Wait(%v) returned error: %v", sig, err)
		}
		if got := sh.Signal(); got != sig {
			t.Errorf("Signal() = %v, want %v", got, sig)
		}
	}
}

func TestPackageWait(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := Wait(ctx); err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
}

func TestWithSignals(t *testing.T) {
	t.Parallel()

	var got []os.Signal

	sh := New(WithSignals(syscall.SIGHUP), func(s *Shutdown) {
		s.notify = func(_ chan<- os.Signal, sigs ...os.Signal) { got = sigs }
		s.stopNotify = func(chan<- os.Signal) {}
	})
	sh.End()

	if err := sh.Wait(context.Background()); err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}

	if len(got) != 1 || got[0] != syscall.SIGHUP {
		t.Fatalf("watched signals = %v, want [%v]", got, syscall.SIGHUP)
	}
}

func TestReasonString(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
			t.Parallel()

			if got := exitCode(tc.sig); got != tc.want {
				t.Errorf("exitCode(%v) = %d, want %d", tc.sig, got, tc.want)
			}
		})
	}
}

type fakeSignal struct{}

func (fakeSignal) String() string { return "fake" }
func (fakeSignal) Signal()        {}
