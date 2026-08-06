# Security Policy

## Supported versions

| Version | Supported |
|---|---|
| `v3.x` (`github.com/efureev/go-shutdown/v3`) | ✅ |
| `v2.x` | security fixes only |
| `v1.x` | ❌ |

## Reporting a vulnerability

Please report privately rather than in a public issue, through
[GitHub Security Advisories](https://github.com/efureev/go-shutdown/security/advisories/new).

Useful details: the affected version, a minimal reproduction, and what an
attacker gains. Expect an initial response within a few days; this is a
small project maintained in spare time, so a fix may take longer than the
acknowledgement.

## Scope

The package has no external dependencies — its entire supply chain is the Go
standard library — and it performs no I/O, parsing, or network access. The
realistic security-relevant surface is therefore narrow:

- process termination behavior, including the force quit that calls `os.Exit`
  on a second signal;
- cleanup hooks not running, or running after their deadline, when that would
  leave sensitive state unflushed or a resource open;
- the exit code reported to an orchestrator.

Bugs in hooks supplied by the calling application are out of scope.

`govulncheck` runs on every push and pull request.
