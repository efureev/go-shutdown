package shutdown

import (
	"fmt"
	"os"
	"syscall"
)

func Example_with_logger() {

	logger := new(mockLogger)
	go fn(syscall.SIGTERM)
	err := WaitWithLogger(logger, syscall.SIGINT, syscall.SIGTERM)

	fmt.Fprintln(os.Stdout, logger.Logs[0])
	fmt.Fprintln(os.Stdout, logger.Logs[1])
	fmt.Fprintln(os.Stdout, err)

	// Output:
	// shutdown started...
	// shutdown complete...
	// <nil>
}
