package shutdown

import (
	"log/slog"
	"os"
	"time"
)

// Option configures a Shutdown at construction. Everything an Option touches
// is immutable afterwards, so configuration cannot race with a running Wait.
type Option func(*Shutdown)

// WithLogger sets the logger used to report the shutdown sequence. A nil
// logger keeps the default, which discards everything.
//
// The package logs at three levels: Info for the start and the clean finish,
// Debug for each hook that completed, Error for each hook that failed.
func WithLogger(l *slog.Logger) Option {
	return func(s *Shutdown) {
		if l != nil {
			s.log = l
		}
	}
}

// WithTimeout limits the time allowed for the whole hook sequence. A
// non-positive duration (the default) means no limit.
//
// When the budget runs out, Wait returns an error wrapping ErrTimeout that
// names the hooks still running, and the context passed to the hooks is
// canceled. Those hooks keep running in their own goroutines until they
// return, so long cleanup must honor ctx to be interruptible.
func WithTimeout(d time.Duration) Option {
	return func(s *Shutdown) { s.timeout = d }
}

// WithSignals replaces the watched signals. The default is SIGINT, SIGTERM
// and SIGQUIT. Passing none keeps the default.
func WithSignals(sigs ...os.Signal) Option {
	return func(s *Shutdown) {
		if len(sigs) > 0 {
			s.signals = sigs
		}
	}
}

// WithForceOnSecondSignal controls what happens when a watched signal arrives
// while the hooks are still running. When enabled (the default), the process
// terminates immediately with exit code 128+signum, giving the operator a way
// out of a hung cleanup — the behavior they would get from the OS if the
// process did not handle signals at all.
//
// Disable it only when cleanup must never be interrupted; note that doing so
// leaves SIGKILL as the only way to stop a hook that ignores its context.
func WithForceOnSecondSignal(v bool) Option {
	return func(s *Shutdown) { s.force = v }
}
