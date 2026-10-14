# elg

<img src="assets/elg.png" alt="elg logo: a blue and silver moose with a falcon head" width="320">

An independent Go CLI for administering GleSYS email hosting, DNS and servers.
For humans, shell scripts and coding agents. **This is not an official GleSYS
product.** GleSYS is the provider's trademark.

## Installation

Download the archive for your platform from [Releases](https://github.com/activout/elg/releases).
Check it against the release's `checksums.txt`, extract it, and put `elg` (or
`elg.exe`) on your PATH. Linux and macOS support amd64 and arm64; Windows supports
amd64. Run `elg version --json` to inspect the build and SDK pin.

From v0.2.0, release archives and their unpacked executables have GitHub build
provenance attestations. Verify either before running the binary:

```sh
gh attestation verify elg_v0.2.0_darwin_arm64.tar.gz --repo activout/elg
# After extraction:
gh attestation verify ./elg --repo activout/elg
```

These attestations establish GitHub build provenance; Apple signing and
notarisation are separate. For macOS's download quarantine warning, see
[Apple's instructions](https://support.apple.com/en-us/102445).

The unrelated European Language Grid Python package also installs an `elg`
command. If you use both, install this binary under a different local filename.

To build from source, use Go **1.26 or newer**:

```sh
git clone https://github.com/activout/elg.git
cd elg
go build -o elg ./cmd/elg
```

The SDK fork is pinned in `go.mod`; no sibling checkout or local `replace` is
needed. Build from a checkout rather than `go install ...@version`, which rejects
dependency replacement directives. `go install ./cmd/elg` inside the checkout works.

## Go SDK

elg uses the [official GleSYS Go SDK](https://github.com/glesys/glesys-go),
`github.com/glesys/glesys-go/v8`, through our
[fork](https://github.com/activout/glesys-go). The fork retains the upstream module
path, MIT licence and existing public methods. It adds capabilities and fixes
needed by the CLI:

- **Generic API requests:** `Client.Request` uses the existing SDK transport with
  validated relative endpoints and scalar GET parameters. It preserves complete
  JSON responses, including unknown fields and numeric precision.
- **Per-client HTTP configuration:** `Client.SetHTTPClient` supports custom
  transports and timeouts without changing the global HTTP client. The default
  client refuses redirects.
- **Structured errors:** `APIError` exposes HTTP status, provider status/message
  and `Retry-After`, including provider failures returned with HTTP 200.
- **Explicit email settings:** `EmailAccountPatch` and `EditAccountPatch` preserve
  supplied zero, disabled and empty values instead of treating them as omitted.
  `CreateAccountWithSettings` follows the documented creation API without requiring
  the older SDK's undocumented password argument.
- **Accurate pricing responses:** `CostsDetails` preserves pricing field types and
  decimal precision. This handles numeric quota amounts returned by the API that
  the upstream SDK's string field cannot decode.
- **Explicit DNS and server edits:** `DNSRecordPatch` and `ServerPatch` distinguish
  omitted values from supplied zero or empty values. The CLI can move a DNS record
  to the zone apex, clear a server description or clear backup schedules.
- **Response handling:** response bodies are always closed, reads are limited to
  16 MiB, and malformed or empty success JSON is rejected.
- **Escaped overview filters:** email overview filters are safely encoded in API
  request paths.

See [SDK and API coverage](docs/sdk-coverage.md) for the pinned fork version,
method mapping and verification limits.

## Credentials and profiles

```sh
# Set these through your shell or secret manager, never as CLI key arguments.
export ELG_PROJECT=CL12345
export ELG_API_KEY='your-api-key'
elg email overview
```

`ELG_PROJECT_ID` is also accepted when `ELG_PROJECT` is unset. elg does not load
`.env` files automatically.

For several projects:

```sh
elg config profile set personal --project CL12345 --api-key-env PERSONAL_API_KEY
elg config profile use personal
elg config profile list
elg config show
elg --profile personal email overview
elg config profile remove personal
```

Profiles store an environment variable **name**, never its credential value.
Configuration lives in the standard per-user configuration directory under
`elg/config.json` (`os.UserConfigDir`: e.g. `~/Library/Application Support/elg`
on macOS, `$XDG_CONFIG_HOME/elg` on Linux and `%AppData%\elg` on Windows).
Use `--config` or `ELG_CONFIG` to select another file.

Selection is independent for each setting:

- Profile: `--profile`, `ELG_PROFILE`, saved default.
- Project: `--project`, `ELG_PROJECT`, `ELG_PROJECT_ID`, profile project.
- Key: selected profile's configured variable, otherwise `ELG_API_KEY`. A missing
  configured variable is an error; no different project's key is substituted.
- Output: human-readable by default; explicit `--json` or `--ndjson` only.
- Timeout: `--timeout`, `ELG_TIMEOUT`, 30 seconds for the whole API command.

A profile can explicitly set `--api-base-url https://api.example.test`. Only an
HTTPS origin without credentials, path, query or fragment is accepted. Redirects
are refused. There is no key argument flag or plaintext key storage.

## Commands

```sh
elg email overview --filter example.com --json
elg email overview --page 2 --json
elg email list example.com
elg email account list example.com --json | jq '.data[].emailaccount'
elg email account get david@example.com --json
elg email account quota david@example.com --json
elg email account update david@example.com --antispam-level 0 --autorespond=false
elg email account update david@example.com --autorespond-message=''
elg email alias create info@example.com --to david@example.com
elg email alias update info@example.com --to support@example.com --dry-run
elg email alias delete info@example.com --yes --json
elg email costs --json

elg dns domain list
elg dns domain add example.com
elg dns domain get example.com
elg dns domain export example.com --json | jq -r '.data.zonefile'
elg dns record list example.com
elg dns record create example.com --type A --host www --data 192.0.2.1 --ttl 300
elg dns record update 123 --data 192.0.2.2
elg dns record delete 123 --yes
elg dns domain delete example.com --yes

elg server list
elg server get SERVER_ID
elg server templates
elg server create --hostname demo --datacenter Falkenberg --platform KVM \
  --template TEMPLATE_ID --cpu-cores 2 --memory-mib 2048 --disk-gib 20 \
  --username cloud --public-key-file ~/.ssh/id_ed25519.pub --dry-run
elg server update SERVER_ID --memory-mib 4096 --description ''
elg server start SERVER_ID
elg server stop SERVER_ID --dry-run
elg server stop SERVER_ID --type soft --yes
elg server delete SERVER_ID --yes --keep-ip
```

Use each command's `--help` for supported flags. Server creation requires explicit
sizing, datacenter, platform, hostname and template. Bandwidth defaults to 100 Mbps
and IPv4/IPv6 to `any`; all appear in dry-run. Provisioning accepts public-key,
root-password, cloud-config, template-parameter, user and backup-schedule files.
KVM requires users with credentials or a cloud-config template. Backup schedules
use `frequency` (`daily`/`weekly`) and positive `numberofimagestokeep`; `[]` clears
schedules on update. Additional provisioning can use documented generic API calls.

DNS hosting never registers or renews a paid domain. Zone deletion never forces
email deletion. `dns domain add` creates no default records unless
`--create-records` is supplied, apart from the provider's automatic NS records.
For DNS records, `--host=''` or `--host @` selects the zone apex; the CLI sends `@`
because the API rejects an empty host. Creation requires `--host`; omitting it
during an update leaves the host unchanged.
For MX, put priority and destination in `--data`
(e.g. `"10 mail.example.com."`); SRV data contains priority, weight, port and target.
Other record data follows the provider's syntax. TTL is positive seconds.

## Mutations and passwords

Creates and updates execute when explicitly requested. Deletion, server stop and
password reset require terminal confirmation or `--yes`. `--non-interactive`
never prompts. `--dry-run` validates locally and returns a redacted plan; it may
read resources but does not mutate, reset passwords or reserve secret files.
It is not a provider-side transaction preview.

```sh
elg email account create new@example.com --quota-gib 2 \
  --secret-output-file initial-password.txt --non-interactive
elg email account reset-password david@example.com --yes --non-interactive \
  --secret-output-file new-password.txt --json
```

Before a password-generating request, supply a new `--secret-output-file` or
deliberately use `--show-secrets`. Interactive execution can offer either route.
Files are reserved before mutation, never overwrite existing files and use mode
0600 on systems supporting Unix permissions. On Windows, protect the destination
directory using your account's ACLs. Returned passwords are written as exact bytes
without an added newline. Provisioning password files remove **one** trailing LF
or CRLF and preserve all other whitespace. Text files must be UTF-8.

The documented email creation endpoint has no arbitrary password argument; elg
does not expose one. Creation sends documented settings only and captures a
returned password. The password-return behaviour of creation has not been live
tested. If a mutation succeeds but returns no password or delivery fails, elg
reports `secret_delivery_failed`: do not automatically repeat the mutation.
No secret is printed as a fallback. An ambiguous mutation transport failure
reports `mutation_outcome_unknown`; inspect the resource before retrying.

## Output and generic access

Output is human-readable by default, including in pipes and redirected files.
Nested collections appear as labelled sections and tables. Pass `--json` for one
JSON envelope or `--ndjson` for newline-delimited envelopes. These flags are
mutually exclusive; `--pretty` requires `--json`. No colour is emitted.

`--output` and `ELG_OUTPUT` are no longer supported. Scripts written for v0.1.0
must explicitly pass `--json` instead of relying on redirected stdout.

```json
{"schema_version":1,"data":[],"meta":{"command":"email account list","project":"CL12345"}}
```

Lists are arrays, single resources are objects, and successful operations without
resource data return `data: null` with operation and target metadata. SDK field
names, quota units and provider dates are retained. Arrays are ordered by address,
domain or record/server/template ID where no API ordering is required. Overview
fetches every page by default; `--page` selects one. `meta.pages_fetched` and
`meta.complete` describe completeness; provider pagination remains in the data.

NDJSON emits one compact envelope per top-level list item, with the same metadata
on each line. Empty lists emit no lines. Objects (including overview and generic
provider responses) and successful no-data operations emit one envelope. Nested
arrays remain inside their parent object. JSON numbers and secret redaction are
preserved in both machine formats.

```sh
elg email account list example.com --ndjson | jq -r '.data.emailaccount'
```

```sh
elg api call email/quota --data-file quota-request.json --json
printf '%s\n' '{"emailaccount":"david@example.com"}' |
  elg api call email/quota --method POST --data-file - --json
elg api call api/listfunctions --json
```

`quota-request.json` contains `{"emailaccount":"david@example.com"}`.
Generic `data` contains the complete provider envelope, including unknown fields
and exact JSON numbers. GET scalar parameters use the provider's `/name/value`
path convention; nested values require POST. Endpoints must be relative
`service/operation` names. Unknown or mutating operations require `--yes`, even
for GET. Known email password generators also require deliberate secret display;
use the typed commands when a secret file is needed.

Only the small verified read-only endpoint table is retried (up to three attempts,
bounded backoff and Retry-After within the deadline). Mutations are never retried.
Requests are limited to 4 MiB input and 16 MiB responses.

Passwords, tokens, private keys and transfer codes are masked by default, including
nested raw fields. Generic redaction is conservative and key-based; unrecognised
secret fields cannot be guaranteed safe. Review an unfamiliar endpoint before
sharing its output. `--show-secrets` permits result secrets but never the CLI's own
authentication key. Debug logs contain operation names, never request bodies or
authentication headers. Result data goes to stdout; errors/prompts go to stderr.

JSON and NDJSON errors leave stdout empty and write one compact error envelope
to stderr. Default errors are human-readable:

```json
{"schema_version":1,"error":{"code":"not_found","message":"Email account was not found","http_status":404}}
```

| Exit | Meaning |
| --- | --- |
| 0 | Success |
| 1 | Application/API failure, including known secret delivery failure |
| 2 | Invalid input or configuration |
| 3 | Authentication or authorisation failure |
| 4 | Not found |
| 5 | Conflict/precondition |
| 6 | Transport failure/deadline; mutation outcome may be unknown |
| 7 | Rate limit exhausted |
| 8 | Confirmation required/declined |
| 130 | Interrupted; mutation outcome may be unknown |

Help, shell completion and version have specialised output. Generate completion
with `elg completion bash|zsh|fish|powershell`.

## Agent skill

The repository includes an [elg skill](.agents/skills/elg/SKILL.md) for agents using
the CLI, covering profiles, explicit machine formats, mutations and secrets.

## Development and verification

```sh
gofmt -w cmd internal
go vet ./...
go test ./...
go test -race ./...
# Opt-in reads only; never logs resources or credential values:
go test ./internal/cli -run TestLiveReadOnly -v -args -live-env-file=/path/to/.env
bash scripts/release.sh v0.2.0 dist
```

The default test suite requires no credentials. See [SDK coverage](docs/sdk-coverage.md),
[extension guidance](docs/extending.md) and [verification](docs/verification.md).
The release workflow is manually dispatched against the same existing version
tag supplied as its input. For example, after creating and pushing `v0.2.0`:

```sh
gh workflow run release.yml --ref v0.2.0 -f version=v0.2.0
```

The workflow attests the archives and unpacked binaries before publishing.
Email messages, mailbox migration, registrar operations, keyrings, MCP, networking
and other future command groups are outside v0.1.0.
