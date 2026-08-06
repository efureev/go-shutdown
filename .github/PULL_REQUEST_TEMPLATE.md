# What and why

<!-- What changes, and what problem it solves. Link the issue if there is one. -->

## Checklist

- [ ] `go build ./... && go vet ./... && go test -race ./...` pass
- [ ] `golangci-lint run` reports no issues
- [ ] `go mod tidy -diff` is clean and `go.sum` is still empty
- [ ] New behavior is covered by a test that fails without the change
- [ ] Signal-based tests go through the `fakeSignals` seam, not real signals
- [ ] `Readme.md` and `Readme.ru.md` updated together, if documentation changed
- [ ] `CHANGELOG.md` updated

## Public API

- [ ] Unchanged
- [ ] Additive only
- [ ] Breaking — requires a new major version and module path; `MIGRATION.md` updated

<!--
A breaking change needs discussion in an issue first: it forces every importer
to edit their import path.
-->
