package shutdown

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

var signalsDefault = []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT}

type Shutdown struct {
	log     ILogger
	timeout time.Duration

	sigChannel chan os.Signal

	onDestroyFn func()
}

// DefaultShutdown is a default instance.
var DefaultShutdown = New()

func New() *Shutdown {
	sh := &Shutdown{
		sigChannel: make(chan os.Signal, 1),
	}

	return sh
}

func (s *Shutdown) Wait(signals ...os.Signal) {
	if len(signals) == 0 {
		signals = signalsDefault
	}

	signal.Notify(s.sigChannel, signals...)

	<-s.sigChannel

	done := make(chan bool, 1)

	go func(fn func()) {
		defer func() { done <- true }()

		logInfo(s.log, `shutdown started...`)

		if fn != nil {
			fn()
		}
	}(s.onDestroyFn)

	<-done

	logTrace(s.log, `shutdown complete...`)
}

func (s *Shutdown) SetLogger(l ILogger) *Shutdown {
	s.log = l

	return s
}

func (s *Shutdown) OnDestroy(fn func()) *Shutdown {
	s.onDestroyFn = fn

	return s
}

func (s *Shutdown) End() {
	s.sigChannel <- syscall.SIGQUIT
}

func Wait(signals ...os.Signal) {
	DefaultShutdown.Wait(signals...)
}

func WaitWithLogger(logger ILogger, signals ...os.Signal) {
	DefaultShutdown.SetLogger(logger).Wait(signals...)
}

func End() {
	DefaultShutdown.End()
}

func OnDestroy(fn func()) () {
	DefaultShutdown.OnDestroy(fn)
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
