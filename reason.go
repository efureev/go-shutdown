package shutdown

import (
	"os"
	"syscall"
)

// Reason tells what triggered the shutdown.
type Reason int

const (
	// ReasonNone means the shutdown has not been triggered yet.
	ReasonNone Reason = iota
	// ReasonSignal means one of the watched OS signals was received.
	ReasonSignal
	// ReasonContext means the context passed to Wait was canceled.
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
//	err := sh.Wait(ctx)
//	if err != nil {
//		log.Printf("cleanup failed: %v", err)
//	}
//	os.Exit(sh.ExitCode())
func (s *Shutdown) ExitCode() int {
	if sig := s.Signal(); sig != nil {
		return exitCode(sig)
	}

	return 0
}

// exitCode maps a signal to the conventional shell exit code 128+signum, so
// an orchestrator can tell a forced stop from a clean one.
func exitCode(sig os.Signal) int {
	if s, ok := sig.(syscall.Signal); ok && s > 0 {
		return 128 + int(s)
	}

	return 1
}
