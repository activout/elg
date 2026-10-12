First release of **elg**, an independent administration CLI for GleSYS.

- Email domains, accounts, aliases, quotas, settings, costs and safe password reset.
- DNS zones and records; explicit server provisioning and lifecycle commands.
- Profiles referencing environment variables, versioned JSON, stable exit codes,
  confirmation and dry-run plans.
- Complete generic API responses through a pinned SDK fork, with secret redaction
  and exact JSON numbers.
- Single binaries for Linux/macOS amd64 and arm64, plus Windows amd64, with SHA-256
  checksums and dependency licence notices.

Automated HTTP fixture tests and race checks pass. Read-only production smoke
tests pass. No production mutation was used for verification. Read the README's
password handling and the verification document for remaining live limitations.
