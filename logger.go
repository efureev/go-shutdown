package shutdown

// Logger is the optional logging interface used by Shutdown to report
// the progress of the shutdown sequence.
type Logger interface {
	Trace(args ...any)
	Info(args ...any)
}
