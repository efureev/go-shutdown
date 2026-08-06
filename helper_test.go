package shutdown

import (
	"os"
	"sync"
	"testing"
	"time"
)

// fakeSignals replaces the signal source so tests can deliver signals without
// sending real ones to the test process.
//
// The real thing forces every signal test to be serial (a signal is delivered
// to the process, not to a test), to sleep before sending so Wait has time to
// subscribe, and to risk killing the whole test binary when that sleep is
// forgotten. With this seam the tests are deterministic and parallel.
type fakeSignals struct {
	mu        sync.Mutex
	ch        chan<- os.Signal
	ready     chan struct{}
	readyOnce sync.Once
}

func newFakeSignals() *fakeSignals {
	return &fakeSignals{ready: make(chan struct{})}
}

// option wires the fake into a Shutdown.
func (f *fakeSignals) option() Option {
	return func(s *Shutdown) {
		s.notify = f.notify
		s.stopNotify = f.stop
	}
}

func (f *fakeSignals) notify(ch chan<- os.Signal, _ ...os.Signal) {
	f.mu.Lock()
	f.ch = ch
	f.mu.Unlock()

	f.readyOnce.Do(func() { close(f.ready) })
}

func (f *fakeSignals) stop(chan<- os.Signal) {
	f.mu.Lock()
	f.ch = nil
	f.mu.Unlock()
}

// send delivers a signal, waiting until Wait has subscribed. It must be called
// from the test goroutine.
func (f *fakeSignals) send(t *testing.T, sig os.Signal) {
	t.Helper()

	select {
	case <-f.ready:
	case <-time.After(2 * time.Second):
		t.Fatal("Wait never subscribed to signals")
	}

	f.mu.Lock()
	ch := f.ch
	f.mu.Unlock()

	if ch == nil {
		t.Fatal("signal handler was already stopped")
	}

	select {
	case ch <- sig:
	case <-time.After(2 * time.Second):
		t.Fatal("nobody read the delivered signal")
	}
}

// waitErr reads a result produced by a Wait running in another goroutine.
func waitErr(t *testing.T, ch <-chan error) error {
	t.Helper()

	select {
	case err := <-ch:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return")

		return nil
	}
}
