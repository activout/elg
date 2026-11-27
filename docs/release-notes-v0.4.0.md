## Changes

- New `elg volume` commands (alias `elg blockstorage`) for GleSYS Block Storage:
  `list`, `get`, `tiers`, `cost`, `create`, `rename`, `resize`, `attach`,
  `detach` and `delete`. Detach and delete require confirmation.
- `email overview --filter` now rejects filters containing `/ ? # % \` or equal
  to `.` or `..` with an invalid-input error (exit code 2), before any request.
  The API does not unescape filters, so such values could never match a domain.
- `elg api call` now builds its requests itself instead of through the SDK, using
  the same HTTP client as typed commands (retries, timeout, no redirects). Raw
  calls send the User-Agent `elg/<version>`.
- Update the GleSYS SDK fork to the reworked upstream proposals
  glesys/glesys-go#145 (response handling) and #146 (configurable HTTP client).
  The fork no longer carries `Client.Request` or overview filter escaping.
- Update golang.org/x/term to 0.48.0.
