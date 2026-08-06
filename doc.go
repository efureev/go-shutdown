/*
Package shutdown provides graceful shutdown for apps and services.

It waits for OS signals (by default SIGINT, SIGTERM and SIGQUIT), for the
cancellation of a context, or for a manual End, and then runs the registered
cleanup hooks before the process exits.

The simplest form, when no cleanup is needed:

	package main

	import (
		"context"
		"log"

		"github.com/efureev/go-shutdown/v3"
	)

	func main() {
		//..

		if err := shutdown.Wait(context.Background()); err != nil {
			log.Fatal(err)
		}
	}

With cleanup, subsystems are torn down in the reverse order they were
registered, and hooks marked Parallel run together:

	sh := shutdown.New(shutdown.WithTimeout(15 * time.Second))

	sh.Add("db", func(context.Context) error { return db.Close() })
	sh.Add("cache", func(ctx context.Context) error { return cache.Flush(ctx) }, shutdown.Parallel())
	sh.Add("search", func(ctx context.Context) error { return search.Flush(ctx) }, shutdown.Parallel())
	sh.Add("http", func(ctx context.Context) error { return srv.Shutdown(ctx) })

	// teardown: http, then cache and search together, then db
	if err := sh.Wait(context.Background()); err != nil {
		log.Printf("cleanup failed: %v", err)
	}

	os.Exit(sh.ExitCode())

Workers can observe the shutdown without blocking in Wait, through Done or
Context.

For a full guide visit https://github.com/efureev/go-shutdown
*/
package shutdown
