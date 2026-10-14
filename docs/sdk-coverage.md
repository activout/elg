# SDK and API coverage

Verified on 12 October 2026 against the official SDK, official API wiki and the
public `api/listfunctions` response. Latest official release: **v8.5.0**.
Selected base: `a25ba1b2ddb53c61a9cb1fc53888c98b9ac0b67e` (SDK version 8.5.0,
module `github.com/glesys/glesys-go/v8`, MIT licence).

Pinned fork: `activout/glesys-go`, commit
`d652253ae971aa763236fa7896318ab80f7662e0`, pseudo-version
`v8.5.1-0.20261012191530-d652253ae971`. Official imports are retained with a pinned
module replacement, not a moving branch or local path.

Sources: [SDK](https://github.com/glesys/glesys-go),
[releases](https://github.com/glesys/glesys-go/releases),
[API wiki](https://github.com/GleSYS/API-docs/wiki/API-Documentation),
[public endpoint documentation](https://api.glesys.com/api/listfunctions/format/json).
The adapted [specification](specification.md) records the requirements and owner decisions.

All commands below are implemented and have local HTTP exchange tests. Read-only
verification and limited live mutation tests are recorded separately below;
untested mutations remain unverified.

| CLI command | SDK service/method | Endpoint | Classification / gaps |
| --- | --- | --- | --- |
| email overview | EmailDomains.Overview | email/overview | Read; aggregate all pages or one explicit page |
| email list | EmailDomains.List | email/list | Read; account and alias arrays |
| email account list/get | EmailDomains.List | email/list | Read; exact local part and case-insensitive domain for get |
| email account create | EmailDomains.CreateAccountWithSettings (addition) | email/createaccount | Create; only documented settings; returned password delivery required |
| email account update | EmailDomains.EditAccountPatch (addition) | email/editaccount | Update; zero/empty values retained |
| email account delete | EmailDomains.List + Delete | email/delete | Delete; exact account type checked; confirmation |
| email account quota | EmailDomains.Quota | email/quota | Read; usedquotainmib and quotaingib preserved |
| email account reset-password | EmailDomains.List + ResetPassword | email/resetpassword | Mutation; confirmation and secret destination |
| email alias list/get | EmailDomains.List | email/list | Read; exact alias type/address for get |
| email alias create | EmailDomains.CreateAlias | email/createalias | Create; one destination |
| email alias update | EmailDomains.EditAlias | email/editalias | Update; replace destination |
| email alias delete | EmailDomains.List + Delete | email/delete | Delete; exact alias type checked; confirmation |
| email costs | EmailDomains.CostsDetails (addition) | email/costs | Read; preserve raw price field types and decimals |
| dns domain list | DNSDomains.List | domain/list | Read |
| dns domain get | DNSDomains.Details | domain/details | Read |
| dns domain add | DNSDomains.AddDNSDomain | domain/add | DNS hosting only; default createrecords=no |
| dns domain delete | DNSDomains.Delete | domain/delete | Delete; never force-delete email |
| dns domain export | DNSDomains.Export | domain/export | Read; zonefile inside resource object |
| dns record list | DNSDomains.ListRecords | domain/listrecords | Read; sorted by record ID |
| dns record create | DNSDomains.AddRecord | domain/addrecord | Create; type, host, data and optional positive TTL |
| dns record update | DNSDomains.UpdateRecordPatch (addition) | domain/updaterecord | Update host/data/TTL; omitted values retained; type changes require explicit replacement |
| dns record delete | DNSDomains.DeleteRecord | domain/deleterecord | Delete; confirmation |
| server list/get | Servers.List/Details | server/list, server/details | Read; get includes state |
| server templates | Servers.Templates | server/templates | Read; sorted platform arrays |
| server create | Servers.Create | server/create | Create; explicit selection and file-based provisioning |
| server update | Servers.EditPatch (addition) | server/edit | Update; explicit clearing retained |
| server start | Servers.Start | server/start | Mutation; no extra prompt |
| server stop | Servers.Stop | server/stop | Mutation; soft/hard; confirmation |
| server delete | Servers.Destroy | server/destroy | Delete; confirmation; keep-IP selection |
| api call | Client.Request (addition) | validated service/operation | Complete provider envelope; unknown operations require --yes |

## Decisions and resolved gaps

- The API wiki requires only `emailaccount` on creation and lists no password
  argument. The older SDK requires a `Password` field. The additive creation method
  follows the documented API and sends no arbitrary password. It captures a
  password from the account or top-level response if supplied. Creation with
  returned-password delivery was verified on a disposable mailbox on 14 October
  2026; this does not certify all possible creation settings.
- Explicit numeric zero, boolean `no` and empty message values are preserved by
  presence-aware SDK types. Whether an individual value is accepted is determined
  by the API. The CLI never substitutes zero for an omitted flag.
- Live costs return `pricelist.quota.amount` as a number, contrary to the old SDK
  string model. `CostsDetails` preserves raw pricing JSON instead of changing
  upstream public field types or coercing the output.
- SDK transport now closes every response, bounds reads, exposes structured HTTP
  and provider errors, and escapes overview filters. HTTP 200 is the retained
  success contract; malformed or empty success JSON is rejected.
- GET raw scalar parameters use the documented path convention. Unknown fields and
  numeric precision survive; secret redaction is applied only by the CLI.
- Overview's `total` counts domains (confirmed by upstream fixtures and live
  metadata); `perpage` determines the page count. Changes/repeated domains,
  inconsistent metadata and premature empty pages fail rather than truncate.
- No other mandatory collection showed pagination in the SDK or inspected API
  documentation; the CLI does not invent it.
- Multiple alias destinations, arbitrary password editing, display-name editing,
  saved autoresponses and catch-all settings have no verified interface here and
  are unavailable in typed commands. Unsupported operations can be explored via
  deliberate generic access after checking current provider documentation.

## Live mutation verification: 14 October 2026

Authorised tests used a separate, initially empty project and `ruckel.se`, whose
public DNS remained hosted elsewhere. Each update was followed by an independent
read. Comparisons checked the changed field and the unchanged decoded resource
fields, excluding the email modification timestamp and password.

| Test | Observed result |
| --- | --- |
| Create a 1 GiB mailbox, spam level 3, antivirus enabled, autoresponse disabled | Settings read back correctly; returned password saved to a non-empty private file |
| Update spam level 3 → 0 | Zero retained; other settings unchanged |
| Antivirus enabled → disabled → enabled | Explicit `no` and `yes` retained; other settings unchanged |
| Set and clear autoresponse text | Text saved and cleared; autoresponse remained disabled |
| Change TXT data and TTL 300 → 600 | Each change retained; other record fields unchanged |
| Update DNS host with `--host=''` | Initially rejected with HTTP 400: the API requires a non-empty host. The corrected CLI sends `@`; read-back confirmed the apex and preserved the other fields |
| Create TXT records with `--host @` and `--host=''` | Both read back as `@` |
| Update TTL with host omitted | Existing apex host retained |

The mailbox, password file, test TXT records and newly created zone were removed.
GleSYS added three NS records even with `createrecords=no`; no MX records were
created. Cleanup preserved those NS records until the test zone was deleted.
Server edits, backup clearing, other field combinations and actual mail delivery
remain unverified live. This records the tested behaviour, not every SDK capability.

### Additional tests without billable resource creation

Further tests on 14 October 2026 created only temporary DNS hosting and records.
No mailboxes, servers, registrations or renewals were created.

| Test | Observed result |
| --- | --- |
| Create and independently read A, AAAA, CNAME, MX, SRV and TXT records | Host, type, data and TTL matched for all six types |
| Domain details and zone export | Correct domain returned; exported zone contained all test hosts |
| Generic `domain/listrecords` GET with scalar parameters and POST with JSON parameters | Provider response envelope retained; all six record fields matched the typed SDK list |
| Human default, JSON and NDJSON on a non-empty live list | Default was human-readable; NDJSON row data matched the JSON list |
| Exact lookup of an absent mailbox | Exit 4, `not_found`, empty stdout and a single JSON error on stderr |
| Lookup of an absent DNS zone | Provider HTTP 404 retained; exit 4, `not_found`, empty stdout |
| Server list and templates | Empty server list; 15 KVM and 16 VMware templates decoded without provisioning |

An A → AAAA type/data update was rejected with HTTP 422. A follow-up with data
valid for both TXT and A returned success but retained type TXT, through both
the typed SDK and generic POST. Although the API wiki lists `type` as optional
for updates, the tested API ignored it. The CLI therefore offers `--type` only
on creation and rejects it on updates before any data can be partially changed.
No automatic delete-and-create fallback is performed.

All test records and created zones were removed. The email costs response was
unchanged before and after these additional tests. Server mutation behaviour,
multi-page live pagination and failure modes such as timeouts and rate limiting
remain covered by local fixtures rather than these live checks.

## Verified read-only table and deferred work

Read/retry entries are `email/overview`, `email/list`, `email/quota`, `email/costs`,
`domain/list`, `domain/details`, `domain/export`, `domain/listrecords`,
`server/list`, `server/details`, `server/templates` and `api/listfunctions`.
GET alone is not a safety classification. Mutations have no automatic retry.

Registrar registration/transfers/renewals, IPs, load balancers, databases, storage,
server disks and networking remain deferred typed groups. Raw API access remains
available, without implementing a second transport or an S3 transfer client.
