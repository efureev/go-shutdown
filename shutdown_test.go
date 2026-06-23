package shutdown

import (
	"context"
	"errors"
	"os"
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
		go func() {
			time.Sleep(10 * time.Millisecond)
			End()
		}()

		if err := Wait(); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
	})

	t.Run("on destroy", func(t *testing.T) {
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
