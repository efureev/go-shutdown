package shutdown_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/efureev/go-shutdown/v3"
)

// The usual case: a few subsystems torn down in the reverse order they were
// registered. End stands in for the OS signal that would normally start the
// shutdown.
func Example() {
	sh := shutdown.New()

	sh.Add("db", func(context.Context) error {
		fmt.Println("db closed")

		return nil
	})
	sh.Add("http", func(context.Context) error {
		fmt.Println("http stopped")

		return nil
	})

	sh.End()

	if err := sh.Wait(context.Background()); err != nil {
		fmt.Println("cleanup failed:", err)
	}

	fmt.Println("reason:", sh.Reason())
	fmt.Println("exit code:", sh.ExitCode())

	// Output:
	// http stopped
	// db closed
	// reason: manual
	// exit code: 0
}

// Hooks marked Parallel run together as long as they are registered next to
// each other; a plain hook separates two groups.
func ExampleParallel() {
	sh := shutdown.New()

	done := make(chan string, 2)

	sh.Add("db", func(context.Context) error {
		fmt.Println("db closed last")

		return nil
	})
	sh.Add("cache", func(context.Context) error {
		done <- "cache"

		return nil
	}, shutdown.Parallel())
	sh.Add("search", func(context.Context) error {
		done <- "search"

		return nil
	}, shutdown.Parallel())

	sh.End()

	if err := sh.Wait(context.Background()); err != nil {
		fmt.Println("cleanup failed:", err)
	}

	// cache and search ran together, so their order is not fixed.
	fmt.Println("flushed:", len(done))

	// Output:
	// db closed last
	// flushed: 2
}

// A failing hook does not stop the others, and errors.As reaches the one that
// failed.
func ExampleHookError() {
	sh := shutdown.New()

	sh.Add("db", func(context.Context) error { return errors.New("connection reset") })
	sh.Add("http", func(context.Context) error { return nil })

	sh.End()

	err := sh.Wait(context.Background())

	var hookErr *shutdown.HookError
	if errors.As(err, &hookErr) {
		fmt.Println("failed hook:", hookErr.Hook)
		fmt.Println("cause:", hookErr.Err)
	}

	// Output:
	// failed hook: db
	// cause: connection reset
}

// Workers observe the shutdown through Done, without blocking in Wait.
func ExampleShutdown_Done() {
	sh := shutdown.New()

	stopped := make(chan struct{})

	go func() {
		defer close(stopped)

		<-sh.Done()
		fmt.Println("worker stopped")
	}()

	sh.End()
	<-stopped

	if err := sh.Wait(context.Background()); err != nil {
		fmt.Println("cleanup failed:", err)
	}

	// Output:
	// worker stopped
}
