package shutdown

import (
	"fmt"
	"os"
	"syscall"
)

func Example_with_destroy_func() {

	logger := new(mockLogger)
	go fn(syscall.SIGTERM)

	test := ``
	err := OnDestroy(func() error {
		test = `test`
		return nil
	}).
		SetLogger(logger).
		Wait()

	fmt.Fprintln(os.Stdout, logger.Logs[0])
	fmt.Fprintln(os.Stdout, logger.Logs[1])
	fmt.Fprintln(os.Stdout, err)
	fmt.Fprintln(os.Stdout, test)

	// Output:
	// shutdown started...
	// shutdown complete...
	// <nil>
	// test
}
