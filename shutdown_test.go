package shutdown

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestShutdown(t *testing.T) {

	Convey("Test Shutdown", t, func() {

		sh := New()

		fn := func(s os.Signal) {
			time.Sleep(10 * time.Millisecond)
			p, err := os.FindProcess(os.Getpid())

			if err != nil {
				panic(err.Error())
			}
			err = p.Signal(s)
			if err != nil {
				panic(err.Error())
			}
		}

		Convey("Interrupt by default signals", func() {
			for _, v := range signalsDefault {
				go fn(v)
				err := sh.Wait()

				So(err, ShouldBeNil)
			}
		})

		Convey("Interrupt by exactly signal", func() {
			go fn(syscall.SIGTERM)
			err := sh.Wait(syscall.SIGTERM)

			So(err, ShouldBeNil)
		})

		Convey("Interrupt by other signal", func() {
			go fn(syscall.SIGHUP)
			err := sh.Wait(syscall.SIGHUP)

			So(err, ShouldBeNil)
		})

		Convey("Interrupt by manual call", func() {

			go func() {
				time.Sleep(10 * time.Millisecond)
				sh.End()
			}()

			err := sh.Wait()

			So(err, ShouldBeNil)
		})

		Convey("Interrupt with userFunction wo error", func() {

			go func() {
				time.Sleep(10 * time.Millisecond)
				sh.End()
			}()

			var test = ``
			err := sh.OnDestroy(func() error {
				test = `test`
				return nil
			}).Wait()

			So(err, ShouldBeNil)
			So(test, ShouldEqual, `test`)
		})

		Convey("Interrupt with userFunction with error", func() {

			go func() {
				time.Sleep(10 * time.Millisecond)
				sh.End()
			}()

			err := sh.OnDestroy(func() error {
				return errors.New(`error test`)
			}).Wait()

			So(err, ShouldBeError, `error test`)
		})

		Convey("Interrupt with logger", func() {

			go func() {
				time.Sleep(10 * time.Millisecond)
				sh.End()
			}()

			logger := new(mockLogger)
			err := sh.SetLogger(logger).Wait()

			So(err, ShouldBeNil)
			So(logger.Logs[0], ShouldEqual, `shutdown started...`)
			So(logger.Logs[1], ShouldEqual, `shutdown complete...`)
		})
	})

	Convey("Test Default Shutdown", t, func() {

		fn := func(s os.Signal) {
			time.Sleep(10 * time.Millisecond)
			p, err := os.FindProcess(os.Getpid())

			if err != nil {
				panic(err.Error())
			}
			err = p.Signal(s)
			if err != nil {
				panic(err.Error())
			}
		}

		Convey("Interrupt by default signals", func() {
			for _, v := range signalsDefault {
				go fn(v)
				err := Wait()

				So(err, ShouldBeNil)
			}
		})

		Convey("Interrupt by manual call", func() {

			go func() {
				time.Sleep(10 * time.Millisecond)
				End()
			}()

			err := Wait()

			So(err, ShouldBeNil)
		})

		Convey("Interrupt with userFunction wo error", func() {

			go func() {
				time.Sleep(10 * time.Millisecond)
				End()
			}()

			test := ``
			err := OnDestroy(func() error {
				test = `test`
				return nil
			}).Wait()

			So(err, ShouldBeNil)
			So(test, ShouldEqual, `test`)
		})

		Convey("Interrupt with logger", func() {

			go func() {
				time.Sleep(10 * time.Millisecond)
				End()
			}()

			logger := new(mockLogger)
			err := WaitWithLogger(logger)

			So(err, ShouldBeNil)
			So(logger.Logs[0], ShouldEqual, `shutdown started...`)
			So(logger.Logs[1], ShouldEqual, `shutdown complete...`)
		})
	})
}

func TestShutdownEndSafety(t *testing.T) {

	Convey("End is safe", t, func() {

		Convey("End before Wait does not block and stops Wait", func() {
			sh := New()
			sh.End()

			done := make(chan error, 1)
			go func() { done <- sh.Wait() }()

			select {
			case err := <-done:
				So(err, ShouldBeNil)
			case <-time.After(time.Second):
				t.Fatal("Wait did not return after early End")
			}
		})

		Convey("Repeated End does not block", func() {
			sh := New()

			finished := make(chan struct{})
			go func() {
				sh.End()
				sh.End()
				sh.End()
				close(finished)
			}()

			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("repeated End blocked")
			}

			err := sh.Wait()
			So(err, ShouldBeNil)
		})

		Convey("End after Wait completed does not block", func() {
			sh := New()

			go func() {
				time.Sleep(10 * time.Millisecond)
				sh.End()
			}()

			So(sh.Wait(), ShouldBeNil)

			finished := make(chan struct{})
			go func() {
				sh.End()
				close(finished)
			}()

			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("End after Wait blocked")
			}
		})
	})
}

func TestShutdownTimeout(t *testing.T) {

	Convey("Timeout for OnDestroy", t, func() {

		Convey("Slow destroy returns ErrShutdownTimeout", func() {
			sh := New().
				SetTimeout(20 * time.Millisecond).
				OnDestroy(func() error {
					time.Sleep(500 * time.Millisecond)
					return nil
				})

			go func() {
				time.Sleep(10 * time.Millisecond)
				sh.End()
			}()

			err := sh.Wait()
			So(err, ShouldEqual, ErrShutdownTimeout)
		})

		Convey("Fast destroy completes within timeout", func() {
			sh := New().
				SetTimeout(time.Second).
				OnDestroy(func() error { return nil })

			go func() {
				time.Sleep(10 * time.Millisecond)
				sh.End()
			}()

			So(sh.Wait(), ShouldBeNil)
		})
	})
}

type mockLogger struct {
	Logs []interface{}
}

func (l *mockLogger) Info(args ...interface{}) {
	for _, log := range args {
		l.Logs = append(l.Logs, log)
	}
}

func (l *mockLogger) Trace(args ...interface{}) {
	l.Info(args...)
}
