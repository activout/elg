# Verification

## Local automated checks

CLI and SDK patch suites pass `go vet ./...`, `go test ./...` and
`go test -race ./...` on macOS arm64 with Go 1.27.2. Source is formatted with
`gofmt`. CI repeats checks on Go 1.26 and 1.27, Linux/macOS/Windows and verifies
release builds for all five supported OS/architecture combinations.

Fixtures exercise every mandatory command, including real request paths and
parameter bodies. Tests cover explicit versus omitted settings, pagination and
empty arrays, exact account/alias selection, quota units, cost field types, password
preflight/redaction/no-overwrite/delivery failures, JSON errors and exit codes,
profile precedence, missing credential variables, generic endpoint validation and
large numbers, response closure/limits, malformed HTTP responses, deadlines,
interruption, read-only POST retries and no mutation retries, dry-run and
non-interactive confirmation.

Output regressions also cover human-readable defaults in redirected streams and
across command groups, removal of implicit environment selection, NDJSON list,
empty/single/no-data results, exact numbers, secret redaction, buffered encoding
failures, explicit format conflicts and parser errors in both machine modes.

## Read-only production check, 12 October 2026

An explicitly supplied local environment file provided the project and key. It
was never copied to the repository or printed. `TestLiveReadOnly` passed:

- Email overview, domain email lists, account and alias lists, available exact
  account/alias lookups and account quota.
- Email costs, including the numeric quota price field that exposed an upstream
  decoding incompatibility and is now covered by a regression fixture.
- DNS domain list/details/export and record list for an existing zone.
- Server list, details for an available server, and templates.
- Generic read-only POST to `email/costs`, preserving the provider envelope.

The opt-in smoke test logs command groups only, not resource addresses, identifiers,
passwords or API keys. The default suite requires no live credentials.

## Limits

No live creates, updates, deletion, server lifecycle changes or password resets
were performed. Mutation behaviour is verified using documented contracts and
local HTTP fixtures. Email creation's actual password-return behaviour, provider
acceptance of clearing values, provisioning combinations and alias destination
extensions remain unverified live. Generic redaction cannot recognise arbitrary
new secret field names. File permissions on Windows rely on directory ACLs.
