/*
Package `shutdown` is intended for graceful shutdown for your app/processes.


The simplest way to use `Shutdown`:

  package main

  import "github.com/efureev/go-shutdown"

func main() {
	//..

    shutdown.Wait()
}

For a full guide visit https://github.com/efureev/go-shutdown
*/
package shutdown
