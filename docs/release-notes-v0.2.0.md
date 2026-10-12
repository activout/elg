## Changes

- Human-readable output is now the default for all commands, including pipes and
  redirected files. Nested results appear as labelled sections and tables.
- Explicit `--json` emits the versioned result envelope; `--ndjson` emits one
  compact envelope per top-level list item. Empty lists produce no NDJSON lines;
  single results and no-data operations produce one envelope.
- Add a repository agent skill for elg administration, including safe domain
  removal after its email aliases and mailboxes have been removed.
- Group weekly Dependabot updates for Go dependencies and GitHub Actions.
- Add GitHub build provenance attestations for all five release archives and their
  unpacked executables. Build, attestation and release use the same version tag.

## Migration

Scripts must explicitly pass `--json` or `--ndjson`; redirected stdout no longer
selects JSON. `--output` and `ELG_OUTPUT` have been removed. `--pretty` requires
`--json`, and `--json`/`--ndjson` are mutually exclusive. JSON envelopes, exit codes,
secret redaction and number precision are retained.

## Verification

Release builds run `go vet`, the fixture test suite and race checks. SHA-256
checksums accompany Linux/macOS amd64 and arm64 archives and Windows amd64.
Verify an archive or its extracted executable with
`gh attestation verify FILE --repo activout/elg`.

GitHub attestations do not provide Apple signing or notarisation. Production email
delivery and live mailbox creation/password reset remain unverified.
