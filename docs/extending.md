# Adding an API operation

1. Verify the current provider documentation and selected SDK. Confirm arguments,
   response shape, supported zero/empty values, pagination and mutation semantics.
2. Reuse an existing SDK method. If missing, add endpoint types and a
   context-aware method in the SDK fork, with HTTP fixture tests. Retain error
   handling and body lifecycle; do not implement another HTTP client in elg.
3. Add the Cobra command in its existing resource group. Validate input, keep
   argument presence, use `connect`, and return through the shared output layer.
   Explicit creates/updates execute; deletes, stop and password reset use
   confirmation. Dry-run validates and emits a redacted plan before mutation.
4. Declare read versus mutation. Only documented reads may enter the transport's
   read-only retry table. Never infer safety from HTTP GET or retry ambiguous writes.
5. Declare result secret handling. Reserve a destination before credential
   generation; report known mutation success separately from delivery failure.
   Preserve unknown raw JSON fields and numbers while applying redaction.
6. Test the real CLI-to-SDK-to-HTTP path, invalid input, output/errors, dry-run,
   confirmations and any secrets. Update coverage, help and actual README examples.
7. Commit and push SDK changes first; update the CLI to the exact public fork
   pseudo-version. A release must build without local paths or unpublished branches.

To develop SDK patches locally, temporarily use
`go mod edit -replace=github.com/glesys/glesys-go/v8=../glesys-go`. Restore the exact
remote pin and run `go mod tidy`, `go vet` and tests before committing/releasing.
Upstream contributions are optional; elg does not submit them automatically.
