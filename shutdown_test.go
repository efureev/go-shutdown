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
