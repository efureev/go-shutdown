package shutdown

// ILogger interface
type ILogger interface {
	Trace(args ...interface{})
	Info(args ...interface{})
}
