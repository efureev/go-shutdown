package shutdown

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

var fn = func(s os.Signal) {
	time.Sleep(1 * time.Second)
	p, err := os.FindProcess(os.Getpid())

	if err != nil {
		panic(err.Error())
	}
	err = p.Signal(s)
	if err != nil {
		panic(err.Error())
	}
}

func Example_basic() {

	go fn(syscall.SIGTERM)
	err := Wait(syscall.SIGTERM)

	fmt.Fprint(os.Stdout, err)

	// Output:
	// <nil>
}
