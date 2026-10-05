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
| Implementation | Hand-written. Spec-driven generators (Speakeasy, tfplugingen-openapi) were rejected: they would force the API's shape to change (durations, dashboards as one JSON blob, client-supplied widget ids). |
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
| Retrying a PATCH after an unknown outcome | Allowed, but only for resources whose phase spec confirms a repeated PATCH has no further effect (section 6.3). |

## 3. Architecture

### 3.1 How api-go fits in

The provider is a plain consumer of `apiv3`. HTTP, Bearer auth, pagination,
typed errors (`*apiv3.Error` with `Code`, `FieldErrors`, `RequiredScope`,
`RequestID`), and the request/response types all come from api-go. The
provider adds only Terraform logic: schemas, mapping between state and API
types, plan modifiers, and diagnostics.

```
honeybadger repo                api-go                         provider
openapi/v3/** --bundle--> openapi/bundled.yaml --oapi-codegen--> internal/gen
                                                  apiv3 (hand-written) --> internal/provider
                                 bundled.yaml -------------------------------> spec drift test (8.3)
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
- **One HTTP-level addition.** The provider passes `apiv3` an `http.Client`
  whose transport retries (section 6.3), via `WithHTTPClient`. That
  `http.Client` sets **no** `Timeout`: `Client.Timeout` bounds the whole call,
  including the transport's waits between retries, so it would cut a
  `Retry-After` wait short. The transport applies a deadline to each attempt
  instead (section 6.3).

### 3.2 Depending on api-go

- **Local development:** `honeybadger/mcp/go.work` lists `./api-go`,
  `./honeybadger-mcp-server` and `./terraform-provider-honeybadger`, so the
  provider builds against the local api-go checkout, including commits that
  aren't pushed.
- **go.mod:** until api-go is tagged, require a pseudo-version of the `api-v3`
  branch, as the MCP server does. No `replace` directive: it breaks goreleaser
  and registry builds outside the workspace.
- **Phase 2 prerequisite:** the nullable request types the provider relies on
  (section 5.3) arrived in api-go `9b759df`, which is only local. The current
  pin (`c934ab8`) has the older non-nullable types, so `go mod tidy`,
  `GOWORK=off` builds and standalone CI compile against the wrong contract.
  Before phase 2 lands, Kevin pushes `api-v3` at or after `9b759df` and
  `go.mod` is bumped to that commit. Until then, phase 1 work builds only
  inside `go.work`, and CI waits for the push.
- **Release prerequisite:** the first provider release requires api-go's v3 work
  merged and tagged (planned v0.10.0), and the provider's `go.mod` requiring
  that tag. A release build runs with `GOWORK=off`.

### 3.3 Where the OpenAPI spec fits

- **No code generation in the provider.** The only generated code is api-go's
  `internal/gen`, produced from its vendored copy of the honeybadger spec.
- **The spec checks for drift, not for generating code.** A test in the provider
  reads `openapi/bundled.yaml` from the pinned api-go module and compares it
  with the provider's own schemas (section 8.3). Because api-go was generated
  from that same file, the provider, the client and the API contract can't
  silently disagree.
- **Docs** are generated by `tfplugindocs` from the provider's schemas and
  `examples/`, which is standard for registry providers. Attribute descriptions
  are hand-written; deriving them from the spec is a possible later addition,
  not part of this design.

### 3.4 Repository layout

```
main.go                          providerserver.Serve, protocol 6
internal/provider/
  provider.go                    provider schema, Configure, resource registry
  retry.go                       retry transport (section 6.3)
  diagnostics.go                 apiv3.Error -> diag.Diagnostics
  nullable.go                    types.* <-> nullable.Nullable helpers
  <name>_resource.go             one file per resource, plus _test.go
  <name>_data_source.go          where a data source exists
internal/provider/customtypes/   semantic-equality types (e.g. check-in periods)
internal/specdrift/              spec drift test (section 8.3)
examples/                        per-resource HCL, used by docs and tests
docs/                            generated by tfplugindocs
docs/superpowers/specs/          design specs
terraform-registry-manifest.json protocol version for the registry
.goreleaser.yml                  release builds and signing
GNUmakefile                      test, testacc, docs, lint
LICENSE                          MPL-2.0
```

One `provider` package to start with, as HashiCorp's scaffolding does. Split
only if it grows past what one package holds comfortably.

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
- Environment variable names: see open decision D1.

`Configure`, in order:

1. If either attribute is unknown (e.g. from a value not yet computed), it
   returns an attribute error saying the value must be known when the provider
   is configured, instead of treating it as missing.
2. Falls back to the environment variables for null attributes.
3. Returns an attribute error on `api_token` if no token was found.
4. Builds one `apiv3.Client` and hands it to every resource and data source.

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

| Spec | Terraform |
| --- | --- |
| required in the create input | Required |
| optional, nullable, no server default (null clears) | Optional |
| optional, nullable, null resets to a documented fixed default that doesn't depend on the account or plan | Optional + Computed + schema `Default` set to that value, no `UseStateForUnknown` |
| optional, server picks a value with no fixed default, or the default depends on the account or plan (e.g. `purge_days`) | Optional + Computed + `UseStateForUnknown`; removing it from config keeps the current value (documented per attribute) |
| readOnly | Computed |
| readOnly credential (project `token`, project key value) | Computed + Sensitive |
| path identifier (`project_id`) | Required + `RequiresReplace` |
| writeOnly (secrets) | Optional + Sensitive, kept from config, never read back (section 5.5) |
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
it, and plan and state agree. The drift test checks each `Default` against the
spec's documented default (section 8.3).

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

Computed fields that change on their own and would only add noise to state
(e.g. `fault_count`, `last_notice_at`) are left out, unless a phase spec makes a
case for one.

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
    Optional field is mapped as Optional + Computed instead (section 5.2);
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
  applies: a token that can't see an object gets 404 on delete too.
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

The API has no idempotency key, so the transport resends a request only when
resending can't cause a second effect.

| Failure | POST | GET | PATCH | DELETE |
| --- | --- | --- | --- | --- |
| 429 | retry | retry | retry | retry |
| 503 | **no retry** | retry | retry if verified | retry |
| attempt deadline or dropped connection | **no retry** | retry | retry if verified | **no retry** |
| any other status | no retry | no retry | no retry | no retry |

- **429 is safe for every method.** The rate limit is enforced in
  `authenticate_api!`, a prepended `before_action`
  (`honeybadger/app/controllers/v3/base_controller.rb:520`), so the action never
  runs.
- **POST 503 is not safe.** It can come after the action took effect: an alarm
  create can store the Opticon observer, then time out into a 503.
- **PATCH is retried only where verified.** Each phase spec confirms, per
  resource, that its update controller has no effect beyond storing the values
  (no notifications, no state resets), so repeating the same body is harmless.
  Check-ins are verified: a repeated PATCH doesn't re-trigger `reset_state`.
  Resources that fail the check get no retry after an unknown outcome.
- **DELETE is retried after a 503**, where a repeat answers 404, which counts
  as success. It is **not** retried after a deadline: a project delete cascades
  synchronously through its sites, check-ins, dashboards and tokens, and a
  retry would race the delete still running. The error says the delete may
  still be in progress and to run `terraform destroy` again, which then sees
  404.

v3 allows 360 requests per hour per credential owner. A refresh is one GET per
resource, so large configurations can use a meaningful share of that. When the
transport does retry, it:

- waits for `Retry-After` (seconds or HTTP date), or backs off exponentially
  from 1 second without one;
- tries at most 4 times, then returns the last response;
- returns the response straight away if `Retry-After` is over 60 seconds,
  instead of sitting through an hourly reset that looks like a hung apply;
- stops waiting when the request's context ends;
- applies a deadline to each attempt: 30 seconds (matching
  `apiv3.NewClient`'s default), and 5 minutes for DELETE.

The attempt deadline runs until the response body is closed, not until
`RoundTrip` returns: `apiv3` reads the body after the transport hands back the
response, so cancelling on return would fail every successful read. The
transport wraps the response body so `Close` releases the deadline, and
releases it straight away when the attempt fails or is retried. A stall while
reading the body surfaces from `apiv3` as an error and is not retried.

### 6.4 Creates with an unknown outcome

A POST that fails with a 503, hits its attempt deadline, or loses its
connection may or may not have created the object. The provider:

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
| 1. Foundation | Provider config, retry transport, diagnostics, nullable helpers, test harness, release skeleton (manifest, goreleaser, Makefile) | nothing (builds inside `go.work`) |
| 2. Ready resources | `honeybadger_project`, `honeybadger_project_key`, `honeybadger_check_in` | `api-v3` pushed at or after `9b759df` and `go.mod` bumped (section 3.2) |
| 3. Drift guard | `internal/specdrift`, wired into CI, covering phase 2 | phase 2 |
| 4. Contract-dependent | `honeybadger_alarm`, `honeybadger_integration` | honeybadger #6729 and #6730 merged and re-vendored into api-go |
| 5. New api-go services | environments, sites, teams (including project membership), status pages | api-go services written; the sites spec branch for sites |
| 6. Dashboards | `honeybadger_dashboard` | phase 4 conventions settled |
| 7. Release | Production smoke test, GPG signing, registry publishing, docs | api-go tagged; open decisions D3 and D4 |

Each phase starts with its own short spec (schemas, deviations from section 5,
tests), then an implementation plan.

Known API gaps the phase specs must handle:

- **Integrations:** until #6729 merges, creating a check-in or site appends it to
  every integration's lists, so a managed list drifts. Phase 4 uses
  `all_sites`/`all_check_ins`. OAuth types (Slack, GitHub) can be created but
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
  clears or returns to its default), import with a plan showing no changes,
  destroy. Plus a check that a resource deleted outside Terraform is removed
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

Using `resource.UnitTest` and `httptest`, only for behaviour the real API can't
trigger on demand: retry and no-retry decisions per method and status, a
server that sends headers then stalls mid-body, the unknown-outcome create
error, mapping each error code to its diagnostic, and the exact wire body of
clears (explicit `null` for Optional attributes, the default value for
defaulted ones) and omissions. The fake server
is not a general API emulator. Plain Go tests (no Terraform) cover the retry
transport and the period parser.

`resource.UnitTest` runs the Terraform CLI, as acceptance tests do. Locally it
uses `terraform` on `PATH`; CI installs it with `hashicorp/setup-terraform`.

### 8.3 Spec drift test

`internal/specdrift` runs as an ordinary `go test`, with no Terraform binary
and no network:

1. Finds the api-go module directory (`go list -m`) and loads
   `openapi/bundled.yaml`. Outside `go.work` this reads the pinned version, so
   it is only meaningful once that pin includes the contract the provider
   targets (section 3.2).
2. Gets the provider's schemas in-process through `GetProviderSchema`.
3. For each resource, uses a declared mapping (its read schema, create input
   and update input in the spec, plus an explicit list of skipped fields, each
   with a reason) and fails when:
   - a writable spec field is neither mapped nor skipped;
   - a mapped field no longer exists in the spec;
   - a readOnly field isn't Computed, except path identifiers (`project_id`),
     which must be Required and force replacement;
   - a writeOnly field isn't Sensitive;
   - a nullable, optional field can't be cleared (it's Required);
   - a `uniqueItems` array isn't a set.

`GetProviderSchema` returns attribute shapes only: no validators, plan
modifiers or defaults. The remaining checks work around that:

- **Enums:** each resource declares its enum values once, as exported lists in
  its own code. It builds its `stringvalidator.OneOf` validators from those
  lists, and the drift test compares the same lists with the spec's enums. The
  validator's own values are unexported, so they are never inspected directly.
- **Defaults:** read from each resource's framework `Schema` (the attribute's
  `Default` is exported and returns its value) and compared with the spec's
  documented default.
- **`RequiresReplace`:** not inspected. Plan modifiers expose no "requires
  replace" accessor, so unit tests plan a change to each such attribute and
  check that a replacement is planned.

The comparison uses the bundled spec as published, not api-go's
`overlay.yaml`, which holds Go-specific mappings for code generation.

### 8.4 CI

On every push: `go build`, `go vet`, golangci-lint, unit tests (with
`setup-terraform`), the spec drift test, and a check that `tfplugindocs` output
is current, all with `GOWORK=off`. CI is turned on once the api-go pin
resolves to the contract the provider targets (section 3.2). Acceptance tests
run locally until a CI-hosted stack exists; that is out of scope here.

## 9. Open decisions

| # | Decision | Recommendation |
| --- | --- | --- |
| D1 | Environment variable names for token and URL | Read `HONEYBADGER_API_TOKEN` first, then `HONEYBADGER_PERSONAL_AUTH_TOKEN` (the MCP server's name). "Personal" is wrong for `hba_` tokens, and the fallback keeps MCP setups working. URL: `HONEYBADGER_API_URL`. |
| D2 | Local stack setup: which services, start commands, rate-limit override | Kevin or v3-terraform-readiness supplies them; recorded in `CONTRIBUTING.md` in phase 1. |
| D3 | Production test account for the pre-release smoke test | Choose before phase 7. |
| D4 | Registry namespace and GPG signing key | `honeybadger-io`; key owner chosen before phase 7. |
| D5 | Data sources | Only where users reference objects they don't manage: `honeybadger_project` (by name) and `honeybadger_streams` (for alarms). Decided in the phase 2 and phase 4 specs. |
| D6 | Integration shape: one resource per type, or one resource with a typed block per type | Decided in the phase 4 spec. |

## 10. Issues for honeybadger

Server-side problems found while designing the provider. They are fixed in the
honeybadger app or spec, not worked around here.

- **Orphaned Opticon observers.** An alarm create that times out after Opticon
  stored the observer answers 503, and the alarm may not exist in honeybadger,
  leaving an observer no alarm references.
- **Alarm trigger updates that silently do nothing** (section 7), pending
  Opticon's `observer-26-field-threshold` branch.
- **Retried alarm deletes answer 500.** `v3/alarms_controller.rb` deletes the
  Opticon observer, then the alarm. If the request times out between the two,
  a retry gets a 404 from Opticon, which the controller doesn't rescue, so it
  answers 500 and the alarm row stays. Treating an Opticon 404 on destroy as
  already deleted would let the retry finish the local delete.

## 11. Out of scope

- Any API operation that isn't configuration: faults, notices, comments,
  Insights queries, deploys.
- Running the acceptance stack in CI.
- Generating provider code from the spec.
- An idempotency key in the API.
