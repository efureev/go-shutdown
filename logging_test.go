package shutdown

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"syscall"
	"testing"
)

func newTestLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	h := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})

	return slog.New(h), buf
}

// findRecord returns the single log line containing substr, failing the test
// when there is not exactly one. Asserting on a whole dump lets a record at
// the wrong level slip through, because another record supplies the level.
func findRecord(t *testing.T, out, substr string) string {
	t.Helper()

	var found []string

	for line := range strings.SplitSeq(strings.TrimRight(out, "\n"), "\n") {
		if strings.Contains(line, substr) {
			found = append(found, line)
		}
	}

	if len(found) != 1 {
		t.Fatalf("want exactly one record containing %s, got %d:\n%s", substr, len(found), out)
	}

	return found[0]
}

func TestLogging(t *testing.T) {
	t.Parallel()

	t.Run("a clean run reports start and completion", func(t *testing.T) {
		t.Parallel()

		log, buf := newTestLogger()

		sh := New(newFakeSignals().option(), WithLogger(log)).
			Add("db", func(context.Context) error { return nil })

		if err := runNow(t, sh); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}

		out := buf.String()
		for _, want := range []string{
			`msg="shutdown started"`,
			`reason=manual`,
			`msg="hook finished"`,
			`hook=db`,
			`msg="shutdown complete"`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("log does not contain %s\n%s", want, out)
			}
		}
	})

	t.Run("a failing hook is reported at error level", func(t *testing.T) {
		t.Parallel()

		log, buf := newTestLogger()

		sh := New(newFakeSignals().option(), WithLogger(log)).
			Add("db", func(context.Context) error { return errors.New("connection reset") })

		if err := runNow(t, sh); err == nil {
			t.Fatal("Wait returned nil, want the hook error")
		}

		out := buf.String()

		// Assert per record: searching the whole output would pass even if the
		// hook failure were logged at the wrong level, because the summary
		// record below also carries level=ERROR.
		hookRec := findRecord(t, out, `msg="hook failed"`)
		for _, want := range []string{`level=ERROR`, `hook=db`, `connection reset`} {
			if !strings.Contains(hookRec, want) {
				t.Errorf("hook failure record lacks %s:\n%s", want, hookRec)
			}
		}

		summary := findRecord(t, out, `msg="shutdown finished with errors"`)
		if !strings.Contains(summary, `level=ERROR`) {
			t.Errorf("summary record is not at error level:\n%s", summary)
		}

		if strings.Contains(out, `msg="shutdown complete"`) {
			t.Errorf("a failed run reported completion\n%s", out)
		}
	})

	t.Run("the signal is named when there was one", func(t *testing.T) {
		t.Parallel()

		log, buf := newTestLogger()
		fs := newFakeSignals()

		sh := New(fs.option(), WithLogger(log))

		res := make(chan error, 1)
		go func() { res <- sh.Wait(context.Background()) }()

		fs.send(t, syscall.SIGTERM)
		waitErr(t, res)

		if out := buf.String(); !strings.Contains(out, "signal=terminated") {
			t.Errorf("log does not name the signal\n%s", out)
		}
	})

	t.Run("the default logger discards everything", func(t *testing.T) {
		t.Parallel()

		// WithLogger(nil) must not replace the default with a nil pointer.
		sh := New(newFakeSignals().option(), WithLogger(nil)).
			Add("db", func(context.Context) error { return nil })

		if err := runNow(t, sh); err != nil {
			t.Fatalf("Wait returned error: %v", err)
		}
	})
}
