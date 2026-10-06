# Honeybadger Terraform Provider: Design

Date: 2026-10-05
Status: Draft for review (revised after two rounds of Codex and Fable reviews)

This is the umbrella spec. It covers architecture, how the provider relates to
api-go and the OpenAPI spec, the conventions every resource follows, testing,
and the phase order. It does not design individual resource schemas: each phase
gets its own short spec and implementation plan when its dependencies are ready.

## 1. Goal

Let Honeybadger users manage their account configuration (projects, keys,
check-ins, uptime sites, integrations, alarms, dashboards, and so on) as
Terraform code against the v3 API.

Success means, for every resource:

- `apply` creates it, and an immediate second `plan` shows no changes.
- Changing or removing an attribute in config updates the resource in place, or
  replaces it when the API can't change that field.
- `terraform import` adopts an existing object, and a plan right after import
  shows no changes, apart from documented write-only secrets and defaulted
  attributes whose server value differs from the default (section 5.2).
- `destroy` deletes it, and a resource deleted outside Terraform is dropped from
  state instead of failing the run.

## 2. Decisions already made

| Decision | Choice |
| --- | --- |
| Implementation | Resource code is hand-written. Spec-driven provider generators (Speakeasy, tfplugingen-openapi) were rejected: they would force the API's shape to change (durations, dashboards as one JSON blob, client-supplied widget ids). |
| Nothing hand-synced with the spec | Everything the API spec states (fields, types, required/optional, read-only, write-only, nullable, enums, defaults, descriptions) is generated from honeybadger's bundle. What's hand-written is Terraform behaviour, plus a short list of overrides for facts the bundle doesn't state, such as fields the server fills in (sections 3.3, 5.2). |
| Spec source | honeybadger's generated bundle (`bin/rails openapi:bundle`), copied into the provider. api-go doesn't serve the spec (section 3.3). |
| Framework | `terraform-plugin-framework` (protocol 6), not SDKv2. Current stable versions. |
| API client | api-go's `apiv3` package. The provider makes no HTTP calls of its own. |
| License | MPL-2.0. No vendoring. |
| Location | `honeybadger/mcp/terraform-provider-honeybadger`, its own git repo. |
| Module path | `github.com/honeybadger-io/terraform-provider-honeybadger` (registry naming convention). |
| Spec scope | This umbrella spec, then one spec per phase. |
| Testing | Acceptance tests against a local Honeybadger stack are the main gate (section 8). |
| Idempotency | The v3 API has no Idempotency-Key, and adding one everywhere was judged too much work. The provider is built so it never needs one (sections 6.3 and 6.4). |
| Read 404 | Means deleted: the resource is removed from state (section 5.3). |
| Project team | `honeybadger_project` has no `team_id`; team membership belongs to the teams resource (phase 5). |
| Omitted defaulted attribute | Means the default. Config is the source of truth, so leaving out an attribute with a fixed default sets it to that default (section 5.2). |
| Retries | api-go's `apiv3` owns them, as the opt-in `WithRetry` policy shared with the MCP server. The provider turns it on and has no retry code of its own (section 6.3). |
| Environment variables | Token from `api_token`, then `HONEYBADGER_API_TOKEN`, then `HONEYBADGER_PERSONAL_AUTH_TOKEN` (the MCP server's name); URL from `api_url`, then `HONEYBADGER_API_URL`. Implemented in phase 1 (section 4). |
| Depending on api-go | Develop against the local api-go through `go.work`, as the MCP server does. Only a release needs a tagged api-go (section 3.2). |

## 3. Architecture

### 3.1 How api-go fits in

The provider is a plain consumer of `apiv3`. HTTP, Bearer auth, pagination,
typed errors (`*apiv3.Error` with `Code`, `FieldErrors`, `RequiredScope`,
`RequestID`), and the request/response types all come from api-go. The
provider adds only Terraform logic: schemas, mapping between state and API
types, plan modifiers, and diagnostics.

```
honeybadger                       api-go                                   provider
openapi/v3/** --bundle--+--> openapi/bundled.yaml --oapi-codegen--> internal/gen --> apiv3 --+
                        |         (sha256 recorded)                                           |
                        |               ^ must match                                          v
                        +--> openapi/bundled.yaml --specgen (+ resources.yaml)--> *_resource_gen.go --> internal/provider
```

The provider names generated types only through `apiv3`'s exported aliases. A
type `apiv3` doesn't alias is used through its fields or converted
(e.g. `string(checkIn.State)`), never imported from `internal/gen`.

Rules:

- **Gaps get fixed in api-go, not worked around.** If a resource needs an
  operation `apiv3` doesn't expose (environments, sites, teams and status pages
  today), the service is added to api-go first, following its existing pattern.
  The provider never builds requests itself.
- **Spec problems get fixed at the source.** An awkward or wrong API shape is
  fixed in the honeybadger spec, not hidden in provider code. Provider-side
  handling is limited to Terraform concerns: semantic equality, plan modifiers,
  sensitivity. Server-side problems found along the way go on a list for
  honeybadger (section 10).
- **No HTTP-level additions.** Retries are an `apiv3` client option
  (section 6.3); the provider only turns it on.

### 3.2 Depending on api-go

- **Development uses the local api-go.** `honeybadger/mcp/go.work` lists
  `./api-go`, `./honeybadger-mcp-server` and `./terraform-provider-honeybadger`,
  so the provider builds against whatever is in the local api-go checkout,
  pushed or not, the same way the MCP server does. New api-go features (the
  nullable request types, `WithRetry`) are usable as soon as they're in that
  checkout.
- **go.mod lags until release.** It requires some pseudo-version of api-go's
  `api-v3` branch and isn't bumped as the local checkout moves. A checkout
  outside the workspace, or a `GOWORK=off` build, may therefore fail between
  releases; nothing depends on that. No `replace` directive: it would break
  release builds.
- **A release needs a tagged api-go.** Before a release, api-go's v3 work is
  merged and tagged, the provider's `go.mod` requires that tag, and the release
  builds with `GOWORK=off` (phase 7).

### 3.3 Where the OpenAPI spec fits

The provider generates schema data from the spec, not resource code.

- **Source and release flow:** honeybadger owns the OpenAPI spec. When it
  changes, honeybadger regenerates the bundle (`bin/rails openapi:bundle`,
  `openapi/v3/bundled.yaml`) and the bundle is copied to each downstream
  project: api-go, docs and the provider. Each project regenerates, and if
  anything changed, rebuilds and releases a new version. A problem in the spec
  is fixed in honeybadger and flows down the same way; downstream projects
  never patch their copy.
- **The provider's copy:** `openapi/bundled.yaml`, with `openapi/README.md`
  recording the source branch, commit and sha256, as api-go does for its own
  copy. `make update-spec` copies the bundle from the honeybadger checkout
  (`../../honeybadger`) and records the provenance.
- **Same bundle as the client:** the provider builds on `apiv3`'s types, so a
  spec change that affects them reaches api-go first. `make generate` warns
  when the provider's sha256 differs from api-go's `openapi/bundled.yaml.sha256`
  for the api-go in use (through `go.work`, the local checkout), since
  honeybadger can briefly be ahead of one downstream project; compiling
  against the local `apiv3` and the acceptance tests catch real mismatches. At
  release, `make check-spec-release` fails on a mismatch against the tagged
  api-go. This proves both copies are the same bundle. That
  api-go's generated types match its copy is api-go's own check
  (`make verify-generated`).
- **Generator:** `internal/specgen`, run by `make generate`, loads the copy
  with kin-openapi. kin-openapi is a normal module dependency (the generator is
  a main package in this module), pinned to the version api-go's
  `internal/tools` uses, since the bundle is OpenAPI 3.1. It resolves `$ref`
  and `allOf`, and reads, for each field: type and format, required, nullable
  (3.1 `type: [x, 'null']`, and `oneOf`/`anyOf` null unions), readOnly,
  writeOnly, enum values, default, description, `uniqueItems`, pattern and
  bounds. For each operation it finds the request body schema and the schema of
  the success response's `data` property (responses are `{data, meta}`
  envelopes; deletes have no body), and fails if an operation a resource uses
  doesn't have that shape.
- **Resource config:** `internal/provider/resources.yaml` names, per resource,
  the operations it uses (create, read, update, delete), its import format,
  and the overrides of section 5.2. It's the only hand-written input to the
  generator.
- **Generated code:** for each resource, the generator applies the merge rule
  and table of section 5.2 plus the config's overrides, and writes
  `internal/provider/<name>_resource_gen.go` with the resource's framework
  schema function and its model struct (`tfsdk`-tagged fields, custom types
  where configured). Like `tfplugindocs` output, it's committed and never
  edited. Generation fails when a writable field is neither mapped nor
  omitted, when an override names a field missing from the schema it targets,
  or when the spec has a shape the generator doesn't support yet (nested
  objects, for now). This is HashiCorp's `tfplugingen-framework` idea with
  this design's merge rules; that tool (a tech preview, last released in 2024)
  isn't used.
- **Generated CRUD:** the generator also writes each resource's mapping
  between its model and `apiv3`'s params and response types, which it reads
  with `go/types`, and its CRUD wrapper (create, read, update, delete,
  import, including the unknown-outcome errors of section 6.4), plus the
  provider's resource list. Details: `2026-10-05-phase-2b-resources-design.md`.
- **Hand-written per resource:** the entry in `resources.yaml`, and behaviour
  the spec can't express, such as config validators for side effects
  (section 5.3). A resource with unusual CRUD can opt out with
  `crud: manual`.
- **Descriptions:** a configurable attribute takes its input field's
  description, where constraints live (`cron_timezone`'s input says Rails zone
  names are required); a computed-only attribute takes its read field's. The
  generator sets them as `MarkdownDescription`, since the bundle's descriptions
  use Markdown, and `tfplugindocs` renders them into the registry docs.
  Input descriptions keep sentences that start "Send null to …", because the
  spec is the API reference first. The generator drops those sentences: in
  Terraform, removing the attribute is how a value is cleared or reset, and
  the reset value comes from `default:`. Other advice written for API
  callers (e.g. `report_period`'s "Compare periods by duration, not text")
  stays in the docs for now: accepted as is, with no per-field overrides.

### 3.4 Repository layout

```
main.go                          providerserver.Serve, protocol 6
internal/provider/
  provider.go                    provider schema, Configure, resource registry
  diagnostics.go                 apiv3.Error -> diag.Diagnostics
  nullable.go                    types.* <-> nullable.Nullable helpers
  <name>_resource.go             one file per resource, plus _test.go
  <name>_data_source.go          where a data source exists
internal/provider/customtypes/   semantic-equality types (e.g. check-in periods)
  resources.yaml                 per-resource operations and overrides (section 3.3)
  <name>_resource_gen.go         generated schema and model, per resource
internal/specgen/                the generator (kin-openapi)
openapi/                         bundled.yaml copied from honeybadger, README with provenance
examples/                        per-resource HCL, used by docs and tests
docs/                            generated by tfplugindocs
docs/superpowers/specs/          design specs
terraform-registry-manifest.json protocol version for the registry
.goreleaser.yml                  release builds and signing
GNUmakefile                      test, testacc, docs, lint
LICENSE                          MPL-2.0
```

Resources and helpers share the `provider` package, as HashiCorp's
scaffolding does; the generator and custom types each get their own
package, as listed.

## 4. Provider configuration

```hcl
provider "honeybadger" {
  api_token = "..."                         # optional; env fallback
  api_url   = "https://app.honeybadger.io"  # optional; env fallback, default US
}
```

- `api_token`: sensitive. Accepts `hbt_` personal, `hba_` account, and OAuth
  access tokens, all sent as Bearer.
- `api_url`: defaults to US production. EU is
  `api_url = "https://eu-app.honeybadger.io"`, and a local stack is
  `http://localhost:3001`. The `/v3` segment is optional; `apiv3` adds it.
- No `region` attribute. api-go treats regions as base URLs, and a second way
  to say the same thing isn't worth hard-coding the EU host.
- Environment variables: `HONEYBADGER_API_TOKEN`, then
  `HONEYBADGER_PERSONAL_AUTH_TOKEN`, for the token; `HONEYBADGER_API_URL` for
  the URL. They apply only when the attribute is left out: an explicit empty
  string is an error.

`Configure`, in order:

1. If either attribute is unknown (e.g. from a value not yet computed), it
   returns an attribute error saying the value must be known when the provider
   is configured, instead of treating it as missing.
2. Returns an attribute error for an attribute set to an empty string.
3. Falls back to the environment variables for null attributes.
4. Returns an attribute error on `api_token` if no token was found.
5. Builds one `apiv3.Client`, with `WithRetry` turned on (section 6.3; added
   in phase 2), and
   hands it to every resource and data source.

## 5. Resource conventions

Every resource follows these rules; phase specs only document the deviations.

### 5.1 IDs and import

- IDs are strings everywhere; site ids are UUIDs, still strings.
- Account-level resources (projects, teams, status pages) import by `id`.
- Project-scoped resources import by `project_id/id`. `project_id` is a
  Required attribute that forces replacement, even though the spec marks it
  read-only in response bodies (it is a path parameter on writes).
- `id` is Computed with `UseStateForUnknown`.

### 5.2 Mapping the schema

The generator merges a resource's operation schemas into one attribute set:

1. **Fields:** every field of the read schema, plus path parameters
   (`project_id`). A field only in an input (e.g. `team_id` on project create)
   must be omitted or overridden; generation fails otherwise.
2. **Required or optional:** from the create input's `required` list. The
   update input's own `required` list doesn't make an attribute Required.
3. **Updatable:** a field in the update input can change in place; one only in
   the create input forces replacement.
4. **Clearable:** a field nullable in the update input can be cleared with
   `null` (section 5.3).
5. **Computed:** a read field in neither input, or marked readOnly.
6. Then the table below, then the resource's overrides.

Overrides exist where the bundle doesn't state the fact. Each kind:

- **Omit,** with a reason: input-only fields, and computed fields that change
  on their own and would only add noise to state (e.g. `fault_count`).
- **Sensitive,** as a fallback only. A field the spec marks
  `format: password` (the standard OpenAPI secret marker) is Sensitive without
  an override, whether it's writable, computed or write-only. The `sensitive`
  list covers secrets the spec hasn't marked yet; generation warns when it
  names a field the spec already marks, so the list shrinks to nothing. A
  writeOnly field that is never read back is kept from config if it's a
  secret, and must be omitted otherwise: writeOnly also marks create-only
  flags that simply aren't stored (`disable_email_notifications`).
- **Path identifier** (`project_id`): Required and forces replacement.
- **Refuses to change** (e.g. environment name): forces replacement.
- **Server-filled:** an optional field the server assigns a value to when it's
  omitted, which the bundle doesn't distinguish from one that stays empty
  (`purge_days` versus `language`). A short hand-written list, `purge_days`
  and check-in `slug` ("generated from the name when omitted") today;
  honeybadger chose not to add a spec marker for it, and will revisit if the
  list grows.
- **Custom type** (check-in periods) and **plan modifiers.**

| Spec | Terraform |
| --- | --- |
| required in the create input | Required |
| optional, no default, not server-filled (null clears, if nullable) | Optional |
| optional, with a spec `default:` that doesn't depend on the account or plan, nullable or not (e.g. `schedule_type`, `default: simple`) | Optional + Computed + schema `Default` set to the spec's value, no `UseStateForUnknown` |
| optional and server-filled, or its default depends on the account or plan (e.g. `purge_days`) | Optional + Computed + `UseStateForUnknown`; removing it from config keeps the current value (documented per attribute) |
| readOnly | Computed |
| readOnly ingestion key (project `token`, project key value) | Computed, not Sensitive: they're meant to ship with the application |
| path identifier (`project_id`) | Required + `RequiresReplace` |
| `format: password` | Sensitive |
| writeOnly and `format: password` | Optional + Sensitive, kept from config, never read back (section 5.5) |
| writeOnly, not a secret | Omitted |
| enum | `stringvalidator.OneOf` with the spec's values |
| `uniqueItems: true` array | Set |
| ordered array | List |
| create-only, never returned (e.g. `disable_email_notifications`) | Left out by default. A phase spec may include one only as Optional, never sent on update, with a plan modifier that keeps the prior state value, and documented as having no effect after create. |
| field the API refuses to change (e.g. environment name) | `RequiresReplace` |

Why the fixed-default row: with `UseStateForUnknown`, removing an attribute from
config plans the old value again, so the update sends nothing and the old value
stays. Fields like `disable_public_links`, `resolve_errors_on_deploy`,
`grace_period` and `cron_timezone` reset to a known value on null. Using that
value as the schema `Default` makes removal plan the default, the update sends
it, and plan and state agree. The value comes from the spec's `default:`,
which honeybadger's spec now carries for all four.

Consequences of a `Default`, all intended:

- Leaving the attribute out of config means the default, so it is always
  known and always sent on create.
- After import, an object whose server value differs from the default plans a
  change back to the default. Each resource's import docs list its defaulted
  attributes and tell users to copy the current values into config before
  applying.
- A `Default` is never used where the accepted value depends on the account
  or plan. `purge_days`, for example, answers 403 `feature_unavailable` for
  any value other than the current one on accounts without custom retention,
  so a `Default` would make every create fail there.

### 5.3 Create, read, update, delete

- **Create** sends only known values. Null and unknown attributes are left out
  so server defaults apply; attributes with a schema `Default` are always known
  and always sent. State is set from the create response, not from the plan.
- **Read** maps the GET response into state. A 404 removes the resource from
  state, which tells Terraform it was deleted outside Terraform. The API also
  answers 404 for objects a restricted token can't see. The provider docs
  therefore require the configured token to see every object it manages; a
  narrower token will plan to recreate objects it can't see, and its deletes
  will report success while the object survives (see Delete).
- **Update** relies on the API's PATCH merge. For each writable attribute,
  comparing the plan with the prior state:
  - known value that differs from state: send it;
  - null in the plan with a value in prior state (an Optional attribute removed
    from config): send JSON `null`, which clears it. v3 accepts null only for
    fields marked nullable and rejects it with 422 elsewhere, so a non-nullable
    scalar Optional field is mapped as Optional + Computed instead (section
    5.2). A non-nullable array stays Optional, and removing it plans null;
    whether the update then sends `[]` (clear) or leaves it out is the
    resource's decision, because some fields treat them differently (alarm
    `stream_ids`: omitted means every stream, `[]` means none);
  - unknown: leave it out.

  `apiv3`'s `nullable.Nullable` fields carry all three states.
- **Fields the API changes as a side effect:** some writes change fields the
  request didn't touch. Switching a check-in between `simple` and `cron`
  clears `report_period` or `cron_schedule` and resets `cron_timezone`. The
  plan must predict every such field (in `ModifyPlan`), or the configuration
  that would trigger the change is rejected with a validation error. Either
  way, the state after apply matches the plan. Each phase spec lists its
  resources' coupled fields and which approach each uses.
- **Delete** treats 404 as success. The same token-visibility requirement
  applies: a token that can't see an object gets 404 on delete too. Under
  retries, a delete can also report success for an object that never existed
  (a 503, then 404 on the retry). That's harmless here, because 404 already
  means deleted; import checks existence with a read, never a delete.
- **Whole-value fields:** where the API replaces a field whole (alarm
  `trigger_config`, integration `filters`, dashboard `widgets`), send the full
  planned value, never a merge.

### 5.4 Values the API rewrites

When the API stores a value in canonical form, the attribute uses a custom type
with semantic equality, so an equivalent value doesn't show as a diff. Config
keeps whatever the user wrote.

Check-in periods are the known case. The API accepts ChronicDuration formats
(`<n> <second|minute|hour|day|week|month|year>[s]`, repeated, or `HH:MM:SS`) and
reads them back in ChronicDuration's long form (`1 week` reads as `7 days`; a
zero grace period reads as `00:00:00`). Go's `time.ParseDuration` reads neither
form, so the custom type has its own parser using ChronicDuration's unit table
(a month is 30 days, a year is ChronicDuration's constant). The phase 2 spec
lists the exact unit table and the round-trip cases its tests cover.

### 5.5 Secrets

The API never returns write-only secrets, or returns them masked (e.g.
`og-SEC****`).

- The provider keeps the configured value in state, marks it Sensitive, never
  overwrites it from a read, and sends it only when config changes it.
- Sensitive hides the value in plan output only. It is still stored in state in
  plain text, and the docs say so.
- Import can't recover a secret. The docs say so, and the first apply after
  import sends it.
- Omitting a secret on update keeps it on the server, so removing a secret from
  config needs its own handling. Each phase spec decides, per secret, between:
  - sending the API's clear (null), where the spec allows it;
  - rejecting the removal, where it doesn't. Detecting a value going to null
    needs the prior state, which validators don't see, so this is a plan
    modifier that returns an error diagnostic.

  Forcing replacement is not an option: the replacement would be created
  without the secret it needs. Silently dropping the secret from state while it
  stays on the server is never allowed.

## 6. Errors, retries and creates

### 6.1 Mapping errors to diagnostics

| API error | Diagnostic |
| --- | --- |
| 422 `validation_error` | One attribute error per `FieldErrors` entry, at the path its dotted `field` names (`config.url` maps to that nested attribute). An unknown field becomes a general error. |
| 403 `insufficient_scope` | Error naming the required scope (`RequiredScope()`). |
| 403 `feature_unavailable` / `limit_reached` | Error saying the account's plan doesn't allow it. Kept distinct from access denied. |
| 403 `access_denied` / 401 | Credential error pointing at `api_token`. |
| 422 `ambiguous_account` | Error saying the token covers several accounts and a single-account token is needed. |
| 429 after retries | Error with the rate-limit reset time from `Error.RateLimit`. |
| anything else | General error, including `Error.RequestID` when the response carried one, for support. |

### 6.2 Request ids

`WithRequestIDHook` logs request ids at debug level through `tflog`. This is
best effort: the hook fires only when a response body carries
`meta.request_id`, so successful deletes (empty 204s) log nothing. A failed
request has an id only when its response had a readable body with `meta`;
connection errors and unparseable responses have none. Diagnostics include the
id when there is one (section 6.1).

### 6.3 Retries

Retries belong to api-go: `apiv3`'s opt-in `WithRetry` policy, shared with the
MCP server. Which requests are safe to resend depends on how the server
behaves, so the rules live with the client, not in the provider.

The provider turns the policy on in `Configure` with
`apiv3.RetryPolicy{MaxAttempts: 4, MaxWait: 60 * time.Second}`, and passes no
`http.Client` with a `Timeout` of its own, which would cap every attempt.

What the provider relies on:

- **Creates (POST) are retried only after a 429,** which the server rejects
  before acting. Any other failure leaves the outcome unknown (section 6.4).
- **Updates and deletes are retried** after transient failures. A repeated
  delete that finds nothing counts as deleted (section 5.3).
- **A long wait is reported, not slept through.** v3 allows 360 requests per
  hour per credential owner. When the server asks for a wait longer than the
  maximum, the error comes back straight away instead of sitting through an
  hourly reset that looks like a hung apply.

### 6.4 Creates with an unknown outcome

A POST that fails with a 503, hits its attempt deadline, or loses its
connection may or may not have created the object. So may a 502 or 504 from a
load balancer, and a 2xx whose body
can't be read or has no usable result. A request that was never sent (DNS
failure, connection refused, an untrusted TLS certificate) has a known
outcome: nothing was created. A cancellation doesn't count as never sent: it
can land after the request reached the server. For an unknown outcome, the provider:

- fails the create with an error saying the object may have been created,
  naming the parent (project) and, when the resource has one, the configured
  name or label. Check-ins can be unnamed and project keys unlabelled, so the
  error always also says how to find the object (the list for that project)
  and gives the `terraform import` command with an ID placeholder in the
  resource's format (e.g. `<project_id>/<id>`);
- does not search for the object by name to adopt it automatically: most names
  aren't unique, so it could attach to the wrong object.

In acceptance tests, the `tf-acc-` sweeper removes anything such a failure
leaves behind (section 8.1).

## 7. Phases

| Phase | Contents | Waits on |
| --- | --- | --- |
| 1. Foundation | **Done.** Provider config, diagnostics, nullable helpers, release skeleton (manifest, goreleaser, Makefile, CI, docs) | — |
| 2. Ready resources | 2a: turn on `WithRetry`; the spec copy and checksum check; the generator (section 3.3); the check-in period custom type. 2b: `honeybadger_project`, `honeybadger_project_key`, `honeybadger_check_in`, and the acceptance-test harness | nothing (local api-go) |
| 3. (folded into phase 2) | The drift test it held is replaced by generation (section 8.3) | — |
| 4. Contract-dependent | `honeybadger_alarm`, `honeybadger_integration` | the integration and alarm contract in api-go's vendored spec |
| 5. New api-go services | environments, sites, teams (including project membership), status pages | api-go services written; the sites spec branch for sites |
| 6. Dashboards | `honeybadger_dashboard` | phase 4 conventions settled |
| 7. Release | Production smoke test, GPG signing, registry publishing, docs; `go.mod` bumped to a tagged api-go; `make check-spec-release` passes | api-go tagged; open decisions D3 and D4 |

Each phase starts with its own short spec (schemas, deviations from section 5,
tests), then an implementation plan.

Known API gaps the phase specs must handle:

- **Integrations:** without the `all_sites`/`all_check_ins` flags, creating a
  check-in or site appends it to every integration's lists, so a managed list
  drifts. Phase 4 uses the flags. OAuth types (Slack, GitHub) can be created but
  start inactive until someone connects them in the web UI.
- **Alarms:** an invalid `trigger_config` on update answers 200 and keeps the
  old trigger (Opticon), until Opticon's `observer-26-field-threshold` branch
  lands. `state` lags its history, so it's Computed and never compared.
  `lookback_lag` is always sent.
- **Environments:** notices create them automatically, so a create often hits an
  existing name (422). Phase 5 documents adopting with import.

## 8. Testing

### 8.1 Acceptance tests (the main gate)

- Each resource gets a `TF_ACC` lifecycle test: create, plan with no changes,
  update every mutable attribute, remove every Optional attribute (checking it
  clears or returns to its default), import with a plan showing no changes
  (or, for an object with a non-default value in a defaulted attribute, only
  the documented reset; section 5.2), destroy. Plus a check that a resource deleted outside Terraform is removed
  from state.
- They run locally against a Honeybadger stack. Production is a pre-release
  smoke test only (phase 7).
- They use a dedicated `terraform-acc` account, never a personal dev account,
  on a plan with every feature on and high limits.
- Tokens come from a development-only honeybadger rake task (proposed by
  mcp-v3; honeybadger owns building it). It turns on granular tokens for the
  account, creates an `hba_` token (and an `hbt_` one for user-only
  operations), reuses a fixed name so a rerun rotates the token, and prints the
  token alone on stdout.
- A second, narrower token checks the `insufficient_scope` diagnostic.
- Every test creates its own project and resources, named with a `tf-acc-`
  prefix. A sweeper (`go test -sweep`) deletes any `tf-acc-` objects that
  failed runs left behind, including objects from creates with an unknown
  outcome (section 6.4).
- Alarm tests need Opticon running locally (503 without it).
- To test phase 4 early, the local honeybadger app can run on
  `feat/terraform-3-v3-contract` (after `bin/rails db:migrate`).

### 8.2 Unit tests (only where the real API can't help)

Only for behaviour the real API can't trigger on demand, using `httptest`.
Plain Go tests cover the provider's helpers: error mapping, unknown outcomes
(phase 1), the period parser, and the generator (golden-file tests). `resource.UnitTest` covers a
resource's exact wire body for clears (explicit `null` for Optional
attributes, the default value for defaulted ones) and omissions, and that
each `RequiresReplace` attribute plans a replacement. The fake server is not
a general API emulator. Retry behaviour is tested in api-go, next to the
policy.

`resource.UnitTest` runs the Terraform CLI, as acceptance tests do. Locally it
uses `terraform` on `PATH`; CI installs it with `hashicorp/setup-terraform`.

### 8.3 Staying in step with the spec

There is no separate drift test: the schemas are generated, so they can't
drift. What keeps everything in step:

- **Same bundle as the client:** `make generate` warns, and the release check
  fails, if the provider's copy of the bundle doesn't match api-go's recorded
  sha256 (section 3.3).
- **Generated code is current:** `make generate` followed by a clean
  `git status`. A changed spec copy without regenerating fails.
- **Overrides still match the spec:** generation fails on a writable field a
  resource neither maps nor omits, and on an override naming a field missing
  from the schema it targets. A field the spec drops disappears from the
  generated model, so CRUD code still using it fails to compile.
- **Behaviour the data can't show:** unit tests plan a change to each
  `RequiresReplace` attribute and check that a replacement is planned.
  Acceptance tests (section 8.1) cover the rest against the real API.

### 8.4 CI

The workflow runs `go build`, `go vet`, golangci-lint, unit tests (with
`setup-terraform`), and a check that generated code (spec data and
`tfplugindocs` output) is current, all with `GOWORK=off`. CI can't see the
local api-go, so it is meaningful only once the provider repo has a remote and
`go.mod` requires a pushed api-go, at the latest for a release (section 3.2).
Until then the workflow runs on manual dispatch only, and the same checks run
locally through `go.work`. Acceptance tests run locally until a
CI-hosted stack exists; that is out of scope here.

## 9. Open decisions

| # | Decision | Recommendation |
| --- | --- | --- |
| D2 | Local stack setup: which services, start commands, rate-limit override | Kevin or v3-terraform-readiness supplies them; recorded in `CONTRIBUTING.md` in phase 1. |
| D3 | Production test account for the pre-release smoke test | Choose before phase 7. |
| D4 | Registry namespace and GPG signing key | `honeybadger-io`; key owner chosen before phase 7. |
| D5 | Data sources | Deferred from phase 2b: nothing needs one yet. When added, live data omitted from resources (counts, timestamps, `environments`) belongs there. `honeybadger_streams` is decided in the phase 4 spec. |
| D6 | Integration shape: one resource per type, or one resource with a typed block per type | Decided in the phase 4 spec. |

## 10. API issues the provider runs into

Server-side behaviour that affects the provider. Fixing it is honeybadger's
work; the provider doesn't work around it.

- **Alarm trigger updates can silently do nothing:** an invalid trigger answers
  200 and keeps the old one (section 7).
- **An alarm create that times out can leave an Opticon observer behind** with
  no alarm.
- **A retried alarm delete can fail and leave the alarm undeletable** if Opticon
  already deleted its observer.
- **Secrets marked `format: password`** (done in honeybadger): every
  writeOnly integration secret, `StatusPageInput.password`, and the webhook
  URLs that carry a credential. Never the ingestion keys (`Project.token`,
  `ProjectKey.key`), which aren't secrets.

## 11. Out of scope

- Any API operation that isn't configuration: faults, notices, comments,
  Insights queries, deploys.
- Running the acceptance stack in CI.
- Generating resource code from the spec (schema data is generated; section 3.3).
- An idempotency key in the API.
