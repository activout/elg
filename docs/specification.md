# Accepted implementation requirements

Owner decisions on 12 October 2026 override the original working names:
public `activout/elg`, binary `elg`, MIT licence, `ELG_*` configuration variables
(with `ELG_PROJECT_ID` fallback), separate public SDK fork permitted, read-only
live tests permitted, and publication of v0.1.0 authorised. The known unrelated
`elg` command-name collision was accepted. The source specification below is
adapted for naming and output selection; coverage records API decisions and limitations. Human-readable output is always the default, and only --json or --ndjson selects a machine format.

# GleSYS CLI, code agent specification

## 1. Goal and decisions

Build a practical Go command-line client for administering GleSYS resources, using the official `glesys/glesys-go` library. Email hosting administration is a first-class requirement. The CLI must work equally well for a person at a terminal, shell scripts and coding agents.

The working binary name is `elg` and the working repository name is `elg`. These are implementation defaults, not a claim that the CLI is an official GleSYS product. Use British English in documentation, messages, comments and new identifiers where spelling varies. Preserve existing upstream identifiers and API field names.

Deliver a working CLI, not merely a command skeleton or a generic HTTP wrapper. The first release must include email, DNS, basic server administration, consistent JSON output and an extensibility route for API operations missing from the SDK.

Keep the CLI and SDK as separate Go modules and repositories. Start from the official SDK. A maintained fork is an acceptable way to add missing endpoints or correct limitations. Do not fork solely to obtain JSON output, because the existing exported types already have JSON tags.

This specification authorises implementation and local SDK patches. Creating remote repositories, publishing releases or submitting upstream pull requests requires the owner's repository instructions or explicit authorisation. Prepare changes so those actions are straightforward.

## 2. Evidence and initial discovery

The official SDK source was inspected on 12 October 2026. Its `master` head was `a25ba1b2ddb53c61a9cb1fc53888c98b9ac0b67e`; `go.mod` declared `github.com/glesys/glesys-go/v8`, and `client.go` contained version `8.5.0`. This is an inspected source snapshot, not verification of the latest published release tag.

Source references:

- [Official repository](https://github.com/glesys/glesys-go)
- [Inspected module definition](https://github.com/glesys/glesys-go/blob/a25ba1b2ddb53c61a9cb1fc53888c98b9ac0b67e/go.mod)
- [Inspected HTTP client](https://github.com/glesys/glesys-go/blob/a25ba1b2ddb53c61a9cb1fc53888c98b9ac0b67e/client.go)
- [Inspected email service](https://github.com/glesys/glesys-go/blob/a25ba1b2ddb53c61a9cb1fc53888c98b9ac0b67e/emaildomains.go)
- [Official API documentation](https://github.com/GleSYS/API-docs/wiki/API-Documentation)
- [Current product documentation](https://docs.glesys.com/)

Before implementation, verify the current SDK release, module path, endpoint documentation and licence. Record the exact selected version and commit in `docs/sdk-coverage.md`. Do not copy an old installation example without the module's major-version suffix.

The inspected SDK:

- Uses project ID and API key through HTTP Basic authentication, and accepts an application user agent.
- Exposes typed services, including `EmailDomains`, `DNSDomains`, `Servers`, databases, storage and networking.
- Returns typed resources from its service methods. These can be serialised as JSON, but unknown API fields discarded during decoding cannot be reconstructed by serialising the resource.
- Keeps `get`, `post`, request construction, response handling and the HTTP client field private. A separate CLI package cannot reuse those methods directly.
- Uses `omitempty` on email edit fields. Explicit numeric zero and empty strings can therefore be omitted accidentally.
- Provides email password reset returning a generated password. The inspected `EditAccountParams` does not provide an arbitrary password-setting field.
- Has a response-body lifecycle problem: `parseResponseBody` returns immediately for a nil result before closing the response body. Email deletion follows this path.

The API wiki snapshot and SDK disagree on account creation passwords: the SDK has a required-looking `Password` string field, while the wiki does not list a password argument. Resolve this against current authoritative documentation and, if credentials are available, an explicitly designated disposable test resource. Do not promise arbitrary password setting or invent an endpoint.

Produce a coverage table with SDK service/method, API endpoint, CLI command, mutation classification and any gap. Clearly distinguish implemented, unavailable and deferred functionality. Discovery must lead into implementation, not stop at a report.

## 3. First-release scope

### Email administration, mandatory

Email administration means domains' hosting configuration, accounts, aliases, quotas, spam settings and password reset. Reading messages, sending messages, IMAP, SMTP and mailbox migration are outside this release.

| CLI command | SDK method or implementation route | Required behaviour |
| --- | --- | --- |
| `email overview` | `EmailDomains.Overview` | Domain counts, project limits, filtering and pagination |
| `email list DOMAIN` | `EmailDomains.List` | Accounts and aliases, optional API filter |
| `email account list DOMAIN` | `EmailDomains.List` | Return only accounts |
| `email account get ADDRESS` | Exact selection from `List` | Return one exact account or not-found |
| `email account create ADDRESS` | `CreateAccount` | Create with supported settings and safe password handling |
| `email account update ADDRESS` | `EditAccount` | Patch only explicitly supplied settings |
| `email account delete ADDRESS` | `Delete` | Verify account type, confirm deletion |
| `email account quota ADDRESS` | `Quota` | Preserve used MiB and allocated GiB units |
| `email account reset-password ADDRESS` | `ResetPassword` | Generate new password and deliver it safely |
| `email alias list DOMAIN` | `List` | Return only aliases |
| `email alias get ADDRESS` | Exact selection from `List` | Return one exact alias or not-found |
| `email alias create ADDRESS --to DESTINATION` | `CreateAlias` | Create forwarding alias |
| `email alias update ADDRESS --to DESTINATION` | `EditAlias` | Replace forwarding destination |
| `email alias delete ADDRESS` | `Delete` | Verify alias type, confirm deletion |
| `email costs` | `Costs` | Return costs and price information with currency/units |

Required account flags, where supported by the verified API:

- `--quota-gib`
- `--antispam-level`, validate the documented range, currently 0 to 5
- `--antivirus=true|false`
- `--reject-spam=true|false`
- `--autorespond=true|false`
- `--autorespond-message` or `--autorespond-message-file`, mutually exclusive

Translate CLI booleans to the provider's required representation in the SDK integration. An omitted flag is different from an explicitly supplied `false`, `0` or empty string. Explicit clearing must be transmitted if supported by the API, otherwise return an actionable unsupported-operation error. Do not send a fabricated zero or empty value when no change was requested. If the SDK requires a presence-aware parameter type, implement it there.

Do not expose display-name editing, saved autoresponse options, catch-all behaviour or arbitrary password updates merely because corresponding response fields exist. Add them only when current endpoint support is verified. Alias destination syntax, including multiple destinations, must follow documented provider behaviour.

For exact lookup, derive the domain without altering the local part, call the domain list with the documented filter if appropriate, then verify the returned address and resource type. A fuzzy filter match is not an exact lookup. Preserve internationalised-address behaviour supported by the API; do not impose an unnecessary ASCII-only rule.

### DNS and domains, mandatory

Implement:

- `dns domain list`, `get DOMAIN`, `add DOMAIN`, `delete DOMAIN`, `export DOMAIN`.
- `dns record list DOMAIN`, `create DOMAIN`, `update RECORD_ID`, `delete RECORD_ID`.
- Record flags for type, host/name, data and TTL as supported by the SDK; document any additional required fields per record type.

Keep DNS hosting operations separate from paid registrar operations. Domain registration, transfer, renewal, nameserver changes, auto-renewal and transfer-code retrieval may be added after the mandatory scope. Do not silently register a domain when `dns domain add` was requested.

### Servers, mandatory

Implement `server list`, `get ID`, `templates`, `create`, `update ID`, `start ID`, `stop ID` and `delete ID`, mapped to the verified SDK. Support explicit sizing, location/platform and provisioning settings available in the SDK. Accept cloud-config and public keys through files where supported. Do not invent reboot or backup methods absent from the selected version.

`server create` must require deliberate resource selection, or clearly documented defaults that are visible in a dry-run plan. Do not call upstream `WithDefaults()` without making its effects visible. Existing API types may supply the transport details, but they must not become an opaque requirement to memorise all provider fields for ordinary commands.

### Generic API access, mandatory

Provide `api call ENDPOINT --method METHOD --data-file PATH` as the extensibility escape hatch. Permit `--data-file -` for stdin. Default method is POST; accept GET only when parameters can be encoded according to the documented API convention. `--data-file` is optional for parameterless operations.

This command must reuse the SDK's authentication, user agent, HTTP configuration, error handling and response lifecycle. If upstream lacks a public entry point, add an additive SDK method, rather than duplicate the entire transport in the CLI.

Accept a relative endpoint such as `email/quota`; reject absolute URLs, scheme-relative URLs, traversal and credential-bearing input. Send requests only to the configured API origin. Unknown endpoints remain callable, but classify them as mutations unless present in a small verified read-only endpoint table. Require `--yes` for unknown endpoints, including GET, because an HTTP verb alone does not establish semantics. Known read-only POST endpoints must work without confirmation.

Return the provider response envelope, preserving unknown fields and JSON number precision. This is structurally complete API JSON subject to explicit secret redaction, not a promise of byte-for-byte output. Do not replay a typed command's request just to obtain its raw response.

### Later extensions

IPs, load balancers, databases, object storage, server disks and networking are follow-on typed command groups. Design for these without requiring all of them for the first release. Object-storage administration does not include implementing an S3 transfer client. A dedicated MCP server is not required; the CLI's non-interactive JSON contract must be suitable for one later.

## 4. Output contract

Default to human-readable output for every command, including redirected stdout. Require explicit `--json` for JSON or `--ndjson` for NDJSON; these flags are mutually exclusive. Do not support `--output` or environment-based output selection. Add `--pretty` for indented JSON only; default JSON is compact UTF-8 with a trailing newline. NDJSON uses one compact CLI envelope per top-level list item, no lines for an empty list, and one envelope for objects or no-data operations. Nested collections remain inside their parent object.

Ordinary commands use this versioned envelope:

```json
{"schema_version":1,"data":[],"meta":{"command":"email account list","project":"CL12345"}}
```

- Lists use arrays, single-resource commands use objects, and successful operations without resource data use `data: null` plus `meta.operation` and the target identifier.
- Resource fields preserve the SDK's API JSON names, for example `emailaccount` and `quotaingib`, unless a documented projection is necessary. Do not silently change their units or types.
- `email list` uses an object containing account and alias arrays. `email overview` contains the summary and domains, with pagination described in metadata.
- Empty lists are `[]`, not `null`. Results retain deterministic ordering where the API does not define an order; document the chosen ordering.
- Generic API access uses the same CLI envelope, with the complete provider envelope in `data` and endpoint/method in metadata.
- Dates remain the provider's representation unless explicitly documented. Preserve identifiers and numeric precision, particularly in raw responses; use `json.RawMessage` or `json.Number` appropriately.
- Default output excludes passwords, API keys, access tokens and credential secrets, including nested generic responses. Treat private keys and registrar transfer codes as secrets too. Use schema-aware projections for typed data and conservative recursive key-based redaction for generic data; document that arbitrary unrecognised raw fields cannot be guaranteed secret-free.
- Explicit `--show-secrets` permits requested resource secrets to appear in result data. It never enables printing authentication headers or the CLI's own API key. Use it deliberately for credential-generation operations only.

Write only result data to stdout. Write diagnostics, prompts, warnings and progress to stderr. Disable colour when output is redirected or `NO_COLOR` is set. Buffer and validate JSON before writing so a failure cannot leave a partial success document.

In JSON or NDJSON mode, errors produce no stdout and exactly one JSON object on stderr:

```json
{"schema_version":1,"error":{"code":"not_found","message":"Email account was not found","http_status":404}}
```

Omit unavailable details. Do not infer status codes by parsing arbitrary SDK error strings when an SDK change can expose structured errors. Parser errors must follow this contract when JSON or NDJSON mode is selected, and must not append usage text. Help, completion and version commands have their documented specialised output.

Stable exit codes:

| Code | Meaning |
| --- | --- |
| 0 | Success |
| 1 | Unclassified application/API failure |
| 2 | Invalid input or configuration |
| 3 | Authentication or authorisation failure |
| 4 | Resource not found |
| 5 | Conflict or violated API precondition |
| 6 | Transport failure or deadline exceeded |
| 7 | Rate limit still exceeded after any permitted retry |
| 8 | Confirmation required or declined |
| 130 | Interrupted |

Document `jq` examples against the actual envelope. No built-in query language is required.

## 5. Configuration and credentials

Support named profiles and project selection:

- `config profile set NAME --project CL12345 --api-key-env VARIABLE`
- `config profile list`
- `config profile use NAME`
- `config profile remove NAME`
- `config show`, always redacted

Store configuration in the platform's standard per-user configuration directory. Profiles contain project ID, an environment-variable name for the API key and optional non-secret defaults. Do not store API keys in plaintext in the profile. A keyring integration is optional.

Global options include `--profile`, `--project`, `--config`, `--timeout`, `--json`, `--ndjson`, `--pretty`, `--yes`, `--non-interactive`, `--dry-run` and `--debug`. The timeout default is 30 seconds and applies to the whole command, including pagination and retries. Use a cancellable context, including for SIGINT.

Resolve each setting independently: explicit flag, documented environment variable, selected profile, default. An explicit `--profile` beats `ELG_PROFILE`, which beats the saved default. `--project` overrides `ELG_PROJECT` and the profile project. The API key comes from the selected profile's configured variable, or `ELG_API_KEY` if no such variable is configured. If a configured credential variable is missing, fail instead of falling back to a different account's key. Do not provide a command-line API-key value flag that exposes credentials in process arguments.

Use user agent `elg/VERSION` plus the SDK's own identification. A custom API base URL must be HTTPS, have no embedded credentials and be explicitly configured. HTTP is allowed only for loopback fixture servers in tests. Prevent authenticated redirects from leaking credentials to a different origin. Debug output must be redacted and must not dump request/response bodies by default.

## 6. Mutations and secrets

Routine explicitly requested creates and updates execute without an additional prompt. Deletion, server stop and password reset require confirmation in an interactive terminal, or `--yes`. Show the selected project, target and consequence. Non-interactive execution must never wait for input; missing confirmation returns exit code 8. `--non-interactive` forbids hidden prompts even if stdin is a terminal.

`--dry-run` validates the command and outputs a redacted plan without invoking the mutation or any password reset. Read-only requests may resolve targets and verify prerequisites. Plans include project, operation, target and explicitly changed values. Dry run does not imply the provider will accept the request, and must not be labelled as a server-side transaction preview.

Password input, if accepted by the verified creation API, must use a hidden terminal prompt, `--password-file PATH` or `--password-stdin`. Avoid plaintext password argument flags. Stdin password input and `--data-file -` must be mutually exclusive. Preserve password bytes according to a documented newline policy, and never trim arbitrary whitespace.

For commands that can generate a password, support `--secret-output-file PATH` and explicit `--show-secrets`. In non-interactive mode require a secret destination before performing a password-generating operation. The file contains the secret only, with restrictive permissions, and is created without overwriting an existing file. Check/create the destination before mutation to avoid resetting a password and then discovering an unusable path. In an interactive terminal, offer the same file route or deliberate secret display before changing anything.

If writing the returned secret fails after mutation, report that the mutation succeeded but secret delivery failed. Do not report an ordinary retryable failure or automatically reset again. Never print the secret as an unsolicited fallback. Plaintext memory cannot be guaranteed erasable in Go; do not claim otherwise.

No automatic bulk deletion or production smoke-test mutations. Integration tests use explicitly designated disposable resources and clean up only resources created by the test.

## 7. SDK integration and fork strategy

Use idiomatic Go with Cobra or an equivalently established command parser. Use `encoding/json`. Keep command parsing, API calls and output rendering separate enough for direct testing, without creating a generic plugin system or an interface for every SDK method.

Preferred layout:

```text
cmd/elg/main.go
internal/cli/
internal/config/
internal/output/
internal/sdk/
internal/secrets/
docs/sdk-coverage.md
docs/extending.md
```

The SDK owns endpoint types, authentication, requests and response decoding. The CLI owns argument presence, profiles, confirmation, formatting and output-secret policy. Put new provider endpoints in the SDK, then call them from CLI commands. Do not maintain a second hand-written HTTP implementation in the CLI.

Audit and address the selected SDK's actual limitations. Likely small patches from the inspected snapshot are:

1. An additive public generic request operation returning complete JSON, using the same transport as typed calls.
2. Public HTTP-client configuration/injection for bounded transport behaviour and fixture testing, without replacing global `http.DefaultClient`.
3. A typed API error exposing HTTP status, provider status/message and relevant retry information; preserve wrapping with `errors.Is`/`errors.As`.
4. Presence-aware email edit values that allow explicit zero and empty values. Prefer a compatible additive method or parameter type when changing existing fields would break consumers.
5. Close response bodies on every path, including operations without a decoded result. Bound reads, handle malformed/empty responses according to documented endpoint behaviour, and do not treat arbitrary 2xx responses as equivalent without verification.
6. Correctly encode overview filters instead of concatenating user input into paths without escaping. Preserve the provider's documented URL convention.

Keep changes focused and retain upstream licence notices. Preserve the official import path for CLI source when practical by using a pinned Go module `replace` to the fork. For example, after verifying real versions:

```text
require github.com/glesys/glesys-go/v8 <verified-upstream-version>
replace github.com/glesys/glesys-go/v8 => github.com/<owner>/glesys-go/v8 <verified-fork-version>
```

These are placeholders, not executable dependency declarations. Verify the fork's module declaration, import graph and Go's version rules. An alternative is a deliberately renamed fork module with updated imports; document the trade-off if chosen. Released builds must never depend on a local filesystem `replace`, a moving branch or unpinned `latest`.

A temporary local `replace` or `go.work` is allowed during SDK development. Include a reproducible build script or instructions for the SDK patch checkout until a remote fork is authorised. If no fork is published, the final implementation handoff must still contain all necessary SDK patches and exact base commit references.

Document how to add an API operation: check current provider docs, add request/response types and a context-aware SDK method, test the HTTP exchange, add the CLI command, declare read/mutation semantics, add output and secret rules, update coverage and examples. Keep patches suitable for upstream contribution; submission is optional.

## 8. Pagination, transport and reliability

For email overview, default to fetching all pages, offer `--page N` for one explicit page, and never silently truncate. Aggregate domains once and preserve project-wide summary semantics. Include `pages_fetched` and completion status in CLI metadata. Stop according to documented metadata, protect against repeated pages and malformed pagination, and retain API filters on every request.

Detect actual pagination in other endpoints during discovery and expose it consistently. Do not fabricate pagination for SDK methods returning a complete collection.

Retry only operations verified as read-only, regardless of GET or POST. Use bounded exponential backoff with jitter for relevant transient failures and respect valid `Retry-After` values within the overall deadline. Do not retry authentication/validation errors or mutations without a documented idempotency guarantee. A transport failure after mutation may leave the outcome unknown; state that clearly rather than claiming nothing happened.

Avoid an extra read after a successful mutation unless required by the endpoint contract. Where a multi-step operation partially succeeds, report the known state and return failure; do not perform speculative rollback.

## 9. Verification and distribution

Use local HTTP fixtures, preferably `httptest.Server`, to test the real command-to-SDK-to-HTTP path. Mock provider responses, not every service interface. No live credentials are required for the default test suite.

Mandatory meaningful tests:

- Email overview pagination, empty results, account/alias exact lookup and quota units.
- Create/update/alias/reset/delete request paths and parameter bodies, including omitted versus explicit `0`, `false` and empty values.
- Password creation compatibility decision, reset secret redaction, preflight file creation, no overwrite and failure after mutation.
- JSON success/error envelopes, stable exit codes and clean stdout/stderr separation, including parser failures.
- Profile and environment precedence, missing configured credential variable and redacted configuration/debug output.
- Generic response unknown fields and large numeric identifiers survive intact; endpoint validation prevents origin escape.
- HTTP error classification, non-JSON errors, response-body closure, deadlines and interrupted execution.
- Verified read-only POST retry, no mutation retry, dry-run does not mutate and non-interactive confirmation does not hang.
- One representative DNS write and server lifecycle exchange, proving those groups are real implementations.

Run `gofmt`, `go vet ./...`, `go test ./...` and `go test -race ./...` where supported. Run corresponding checks for SDK patches. Add CI with a current supported Go version and a documented minimum version compatible with the selected dependencies, rather than assuming the SDK's old minimum is ideal for the new CLI.

Provide single binaries for Linux and macOS, amd64 and arm64, and Windows amd64. Include version, build commit and selected SDK version in `elg version`, with JSON support. Supply checksums and a release workflow; do not publish automatically without repository authorisation. Ensure a clean checkout can build without a developer's unpublished local path.

## 10. Examples and completion criteria

The following command syntax must work, subject to valid credentials and provider-supported values:

```sh
elg config profile set personal --project CL12345 --api-key-env ELG_PERSONAL_API_KEY
elg config profile use personal
elg email overview --json
elg email account list example.com --json | jq '.data[].emailaccount'
elg email account get david@example.com --json
elg email account quota david@example.com --json
elg email account update david@example.com --antispam-level 0 --autorespond=false --json
elg email alias create info@example.com --to david@example.com --json
elg email alias update info@example.com --to support@example.com --dry-run --json
elg email account reset-password david@example.com --yes --non-interactive --secret-output-file new-password.txt --json
elg email alias delete info@example.com --yes --json
elg email costs --json
elg dns record list example.com --json
elg server list --json
elg server stop SERVER_ID --dry-run --json
elg api call email/quota --method POST --data-file quota-request.json --json
```

Example `quota-request.json`:

```json
{"emailaccount":"david@example.com"}
```

The work is complete when:

1. Every mandatory command is implemented with working help, validation and the shared output/error contract.
2. Email administration is complete against the verified endpoint set, with no invented password or configuration capabilities.
3. JSON output is suitable for unattended scripting, and raw API access preserves fields unavailable in SDK models.
4. The official SDK integration and any fork patches build reproducibly, with clear dependency pins and licence attribution.
5. SDK gaps, resolved documentation conflicts, supported commands and deferred work are documented accurately.
6. The mandatory tests and build checks pass; no production-changing verification is needed.
7. The README provides installation, profile setup, command examples, `jq` recipes, secret handling and troubleshooting.
8. The implementation handoff states what changed in the CLI and SDK, how it was verified, and which live API behaviours remain unverified without test credentials.

Implement in this order: verify SDK/API and write the coverage table; establish configuration/output/HTTP fixture infrastructure; make the minimal SDK changes; complete all email commands; complete DNS/server commands and generic access; finish tests, documentation and reproducible distribution. Keep the release focused and usable throughout.
