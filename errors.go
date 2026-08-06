package shutdown

import (
	"context"
	"fmt"
)

// ErrTimeout is returned when cleanup did not finish within the configured
// budget, either the whole sequence (WithTimeout) or a single hook
// (HookTimeout).
//
// It wraps context.DeadlineExceeded, so errors.Is reports both.
var ErrTimeout = fmt.Errorf("shutdown: cleanup timed out: %w", context.DeadlineExceeded)

// ErrShutdown is the context.Cause of Context() once the shutdown has started.
// Use Reason for the detail of what triggered it.
var ErrShutdown = fmt.Errorf("shutdown: in progress")

// HookError reports the failure of a single named hook. Wait joins one of
// these per failing hook, so errors.As reaches the individual failure and
// errors.Is still matches whatever the hook returned.
type HookError struct {
	// Hook is the name the hook was registered with.
	Hook string
	// Err is the error the hook returned.
	Err error
}

// Error implements error.
func (e *HookError) Error() string {
	return fmt.Sprintf("shutdown: hook %q: %v", e.Hook, e.Err)
}

// Unwrap gives errors.Is and errors.As access to the underlying error.
func (e *HookError) Unwrap() error { return e.Err }
