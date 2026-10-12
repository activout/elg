---
name: elg
description: Administer GleSYS email accounts and aliases, DNS zones and records, and servers using the elg CLI. Use for elg commands, profiles, dry-run plans, JSON/NDJSON automation or generic GleSYS API access; mailbox messages and migration are outside its scope.
---

# elg

Use the installed `elg` CLI for GleSYS administration. Check `elg version` and
the relevant command's `--help` when flags or capabilities are uncertain. If the
binary is unavailable, build this repository with Go 1.26+ using
`go build -o elg ./cmd/elg`; use `./elg` for that checkout.

## Profiles and output

Use `--profile NAME` or the existing default profile. For direct configuration,
credentials come from `ELG_API_KEY` and project selection from `ELG_PROJECT`
(`ELG_PROJECT_ID` is the fallback). Profiles contain project IDs and credential
environment-variable names, not API-key values:

```sh
elg config profile set personal --project CL12345 --api-key-env PERSONAL_API_KEY
elg config profile use personal
elg config profile list
elg config show
```

Use the user's chosen project/profile; do not guess a production target. Do not
print keys or put them in command arguments. elg does not load `.env` files itself.
A missing configured credential variable fails rather than selecting another key.

Default output is human-readable, including when redirected. **Pass `--json` for
structured automation**, or `--ndjson` for one compact envelope per top-level list
item. The flags are mutually exclusive. `--pretty` requires `--json`. There is no
`--output` flag or `ELG_OUTPUT` selection.

JSON uses `{"schema_version":1,"data":...,"meta":...}`. Lists have array data;
single results have object data. NDJSON list rows each have object data and the
same metadata; an empty list produces no lines. Nested collections, overview and
generic provider envelopes remain inside a single result. Successful operations
without resource data have null data and operation/target metadata.
Errors go to stderr with empty stdout; machine modes use one compact error
envelope. Inspect the exit code before parsing results. Do not parse tables.

## Resource commands

```sh
elg email overview --json
elg email account list example.com --json | jq '.data[].emailaccount'
elg email account list example.com --ndjson | jq -r '.data.emailaccount'
elg email account get david@example.com --json
elg email account quota david@example.com --json
elg email alias list example.com --json
elg email costs --json
elg dns domain list --json
elg dns record list example.com --json
elg server list --json
elg server get SERVER_ID --json
elg server templates --json
```

Overview fetches all pages by default; `--page N` intentionally selects one page.
Check `meta.complete` when completeness matters. Preserve quota units:
`usedquotainmib` is MiB, `quotaingib` is GiB. Exact email lookups preserve the local
part; filtered list results are not an exact-match guarantee.

Scope writes to the operations and resources the user requested. Use `--dry-run`
to review locally validated, redacted changes when helpful. Explicit creates and
updates execute without a second CLI prompt. Deletion, server stop and password
reset require confirmation or `--yes`; unattended commands also use
`--non-interactive`. `--yes` suppresses the CLI prompt, not the user's scope limits.

```sh
elg email account update david@example.com --antispam-level 0 --autorespond=false --dry-run --json
elg email alias update info@example.com --to support@example.com --dry-run --json
elg dns record create example.com --type A --host www --data 192.0.2.1 --ttl 300 --dry-run --json
elg server stop SERVER_ID --dry-run --json
```

Only explicitly supplied settings change; `0`, `false` and an empty autoresponse
message are intentional values. Alias commands accept one forwarding destination.
DNS hosting never registers a domain; domain deletion never force-deletes email.
Server creation requires explicit template, platform, datacenter, hostname and
sizing. Provisioning uses public-key, user, password or cloud-config files;
KVM needs users with credentials or a cloud-config template. Inspect creation help
and a dry-run instead of relying on upstream's hidden defaults.

## Secrets and failure handling

For an authorised email account creation or password reset, reserve a new secret
destination through `--secret-output-file PATH`. Non-interactive generation needs
that option or deliberate `--show-secrets`; prefer the file to exposing a password
in tool output. Files never overwrite existing paths and contain exact returned
bytes with no added newline. There is no arbitrary email password argument.

```sh
elg email account reset-password david@example.com --yes --non-interactive --secret-output-file new-password.txt --json
```

`secret_delivery_failed` means the mutation succeeded but password delivery failed.
`mutation_outcome_unknown` means the response could not establish the outcome.
Do not repeat either mutation automatically; inspect the known state first.
Default redaction covers known secret fields; arbitrary new raw field names cannot
be guaranteed secret-free. Never share unfamiliar provider output blindly.

Exit codes: 2 input/configuration, 3 authentication/authorisation, 4 not found,
5 conflict, 6 transport/deadline, 7 exhausted rate limit, 8 confirmation and
130 interruption. Other application/API errors use 1. Only verified reads retry.

## Missing typed operations

Prefer a typed command. For an operation missing from the CLI, check current
provider documentation and use a relative `service/operation` endpoint through
`api call`, with a JSON object in `--data-file PATH` or `--data-file -` for stdin:

```sh
printf '%s\n' '{"emailaccount":"david@example.com"}' |
  elg api call email/quota --method POST --data-file - --json
```

Generic data contains the complete provider envelope, preserving unknown fields
and exact numbers. POST is the default. GET permits scalar path parameters only.
Unknown/mutating operations require `--yes`, including GET; an HTTP verb alone
does not prove safety. Verify semantics before invoking an unfamiliar endpoint.
See the repository's [README](https://github.com/activout/elg#readme) and
[SDK coverage](https://github.com/activout/elg/blob/main/docs/sdk-coverage.md) for
supported commands and documented gaps.
