package shutdown

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var signalsDefault = []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT}

// ErrShutdownTimeout is returned by Wait when the OnDestroy function
// did not complete within the configured timeout.
var ErrShutdownTimeout = errors.New("shutdown: destroy function timeout")

// Shutdown struct
type Shutdown struct {
	log     ILogger
	timeout time.Duration

	sigChannel chan os.Signal
	done       chan struct{}

	onDestroyFn func() error
}

// DefaultShutdown is a default instance.
var DefaultShutdown = New()

// New creates new instance of Shutdown
func New() *Shutdown {
	sh := &Shutdown{
		sigChannel: make(chan os.Signal, 1),
		done:       make(chan struct{}, 1),
	}

	return sh
}

// Wait waiting for signal
func (s *Shutdown) Wait(signals ...os.Signal) error {
	if len(signals) == 0 {
		signals = signalsDefault
	}

	signal.Notify(s.sigChannel, signals...)
	defer signal.Stop(s.sigChannel)

	select {
	case <-s.sigChannel:
	case <-s.done:
	}

	logInfo(s.log, `shutdown started...`)

	err := s.runOnDestroy()

	logTrace(s.log, `shutdown complete...`)

	return err
}

// runOnDestroy executes the user destroy function (if any).
// When a timeout is configured the function runs in a separate goroutine
// and ErrShutdownTimeout is returned if it does not finish in time.
func (s *Shutdown) runOnDestroy() error {
	if s.onDestroyFn == nil {
		return nil
	}

	if s.timeout <= 0 {
		return s.onDestroyFn()
	}

	resCh := make(chan error, 1)
	go func() { resCh <- s.onDestroyFn() }()

	timer := time.NewTimer(s.timeout)
	defer timer.Stop()

	select {
	case err := <-resCh:
		return err
	case <-timer.C:
		return ErrShutdownTimeout
	}
}

// SetLogger set instance logger
func (s *Shutdown) SetLogger(l ILogger) *Shutdown {
	s.log = l

	return s
}

// SetTimeout limits the time allowed for the OnDestroy function to complete.
// A non-positive duration (default) means no timeout.
func (s *Shutdown) SetTimeout(d time.Duration) *Shutdown {
	s.timeout = d

	return s
}

// OnDestroy apply user-function to execute on app/service terminating
func (s *Shutdown) OnDestroy(fn func() error) *Shutdown {
	s.onDestroyFn = fn

	return s
}

// End triggers the shutdown manually. It is non-blocking and safe to call
// multiple times, before or after Wait.
func (s *Shutdown) End() {
	select {
	case s.done <- struct{}{}:
	default:
	}
}

// Wait is alias for New().Wait(...)
func Wait(signals ...os.Signal) error {
	return DefaultShutdown.Wait(signals...)
}

// WaitWithLogger is alias for New().SetLogger(logger).Wait(...)
func WaitWithLogger(logger ILogger, signals ...os.Signal) error {
	return DefaultShutdown.SetLogger(logger).Wait(signals...)
}

// End is alias for New().End()
func End() {
	DefaultShutdown.End()
}

// OnDestroy is alias for New().OnDestroy(fn)
func OnDestroy(fn func() error) *Shutdown {
	return DefaultShutdown.OnDestroy(fn)
}

func logTrace(logger ILogger, args ...interface{}) {
	if logger != nil {
		logger.Trace(args...)
	}
}

func logInfo(logger ILogger, args ...interface{}) {
	if logger != nil {
		logger.Info(args...)
	}
}
