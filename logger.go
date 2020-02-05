package shutdown

type ILogger interface {
	Trace(args ...interface{})
	Info(args ...interface{})
}
