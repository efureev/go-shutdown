# Contributing

Thanks for taking the time. This is a small, dependency-free library, and the
constraints below exist to keep it that way.

## Ground rules

- **No external dependencies.** `go.sum` is empty and must stay empty. The
  standard library — including `log/slog` — is fine. Anything else is a
  decision for the maintainer, not a side effect of a pull request.
- **The public API is published.** Renaming or changing the signature of an
  exported identifier breaks importers silently on their side. A breaking
  change means a new major version and a new module path (`/v4`), so please
  open an issue before writing that code.
- **Both READMEs stay in sync.** `Readme.md` (English) and `Readme.ru.md`
  (Russian) are translations of one text. Nothing checks that they agree —
  neither the linter nor CI — so a change to one needs the same change to the
  other, in the same place.
- **Update `MIGRATION.md` and `CHANGELOG.md`** when the public surface moves.

## Development

The Makefile runs everything in Docker; a local Go toolchain works directly and
is faster for iteration.

| Task | Locally | Makefile (Docker) |
|---|---|---|
| Build | `go build ./...` | `make build` |
| Vet | `go vet ./...` | `make vet` |
| Tests | `go test -race -timeout 120s ./...` | `make gotest` |
| Lint | `golangci-lint run` | `make lint` |
| Lint + tests | — | `make test` |
| Format | `gofmt -s -w -d . && go mod tidy` | `make fmt` |

`golangci-lint` must be a 2.x release: the config uses the v2 schema, and 1.x
fails while parsing it. The major version is pinned in three places that have
to agree — `.golangci.yml`, the workflow, and the compose image.

Before opening a pull request:

```bash
go build ./... && go vet ./... && go test -race ./... && golangci-lint run
go mod tidy -diff
GOOS=windows go build ./...
```

## Writing tests

- **Do not send real signals to the test process.** Use the `fakeSignals` seam
  in `helper_test.go`. A real signal goes to the process, not to a test: if it
  arrives before `Wait` has subscribed, the default handler kills the entire
  test binary. One integration test (`TestWaitByRealSignal`) covers the real
  path deliberately; it is serial and it is enough.
- **Prove concurrency with a barrier, not with timing.** `TestHooksParallel`
  makes the hooks meet on a `sync.WaitGroup`; a sequential implementation
  deadlocks and fails on the budget. Comparing durations invites flakiness.
- **Assert on a single log record,** via `findRecord`. Searching the whole
  output once let a real defect through, because the level came from a
  neighbouring record.
- **A `Shutdown` instance is single use.** Its hooks run once; a test that
  needs a second shutdown cycle needs a fresh `New()`. `runNow(t, sh)` is the
  usual shortcut.
- **New tests should verify a failure.** Breaking the behaviour under test and
  watching the test go red is the difference between a test that guards
  something and a test that is merely green.

## Commit messages and pull requests

Commit messages are short and in English. The repository does not use
Conventional Commits — please do not introduce it in a single commit.

Work goes through pull requests to `master`. CI runs gitleaks, the linter,
`go mod tidy -diff`, `govulncheck`, a cross-compilation matrix, and the test
suite with `-race` on Linux and macOS.

## Reporting security issues

See [SECURITY.md](SECURITY.md) — please do not open a public issue.
