package shutdown

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

var signalsDefault = []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT}

// Shutdown struct
type Shutdown struct {
	log     ILogger
	timeout time.Duration

	sigChannel chan os.Signal

	onDestroyFn func() error
}

// DefaultShutdown is a default instance.
var DefaultShutdown = New()

// New creates new instance of Shutdown
func New() *Shutdown {
	sh := &Shutdown{
		sigChannel: make(chan os.Signal, 1),
	}

	return sh
}

// Wait waiting for signal
func (s *Shutdown) Wait(signals ...os.Signal) (err error) {
	if len(signals) == 0 {
		signals = signalsDefault
	}

	signal.Notify(s.sigChannel, signals...)

	<-s.sigChannel

	done := make(chan bool, 1)

	go func(fn func() error) {
		defer func() { done <- true }()

		logInfo(s.log, `shutdown started...`)

		if fn != nil {
			err = fn()
		}
	}(s.onDestroyFn)

	<-done
	logTrace(s.log, `shutdown complete...`)

	return err
}

// SetLogger set instance logger
func (s *Shutdown) SetLogger(l ILogger) *Shutdown {
	s.log = l

	return s
}

// OnDestroy apply user-function to execute on app/service terminating
func (s *Shutdown) OnDestroy(fn func() error) *Shutdown {
	s.onDestroyFn = fn

	return s
}

// End send signal for shutting down
func (s *Shutdown) End() {
	s.sigChannel <- syscall.SIGQUIT
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
