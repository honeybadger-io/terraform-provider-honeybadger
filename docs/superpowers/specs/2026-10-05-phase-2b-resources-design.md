# Phase 2b: Project, Project Key and Check-in Resources: Design

Date: 2026-10-05
Status: Draft for review
Parent: `2026-10-05-terraform-provider-design.md` (the umbrella spec). This
spec covers phase 2b only and lists its deviations from the umbrella's
conventions.

## 1. Goal

Ship `honeybadger_project`, `honeybadger_project_key` and
`honeybadger_check_in`. The only hand-written parts are their
`resources.yaml` entries, behaviour the OpenAPI spec can't express, and
tests. Each resource passes a full acceptance lifecycle against a local
Honeybadger stack.

## 2. Decisions

| Decision | Choice |
| --- | --- |
| Field mapping between model and `apiv3` | Generated (section 3.2) |
| CRUD wrappers | Generated (section 3.3), with escape hatches |
| Live-data fields (counts, timestamps, check-in state) | Omitted from the resources |
| `honeybadger_project` data source (umbrella D5) | Deferred; nothing in 2b needs it |
| `token` and project key `key` | Computed, not Sensitive: they're ingestion keys meant to ship with the application |
| Acceptance tests | Written and run in 2b against Kevin's local stack, with a token he provides at run time |

## 3. Generator extensions

### 3.1 Resource config

Each `resources.yaml` entry gains:

```yaml
service: CheckIns                 # the apiv3.Client field
api:
  model: CheckIn                  # apiv3 type a read returns
  create_params: CheckInCreateParams
  update_params: CheckInUpdateParams
find_hint: "the project's check-ins in Honeybadger"   # for the unknown-outcome error
config_validators: checkInConfigValidators            # optional, hand-written
crud: generated                   # or manual, to hand-write the wrapper
```

### 3.2 Generated mapping

The generator loads the real `apiv3` package with
`golang.org/x/tools/go/packages` (`Config.Dir` set to the module root, so
`go.work` resolves the local api-go), and resolves the configured alias names
to their structs. Fields are matched by JSON tag, which equals the spec's
property name; no naming rules are replicated. A model attribute with no
matching field in the struct it needs is a generation error.

Each field's conversion is chosen by its Go type shape:

| Go shape | Write (create / update) | Read |
| --- | --- | --- |
| value (`string`, `bool`, `int`) | `x.ValueString()` etc. on create, for Required attributes only; a create-side value field whose attribute isn't Required is a generation error, since it would be sent as a zero value | `types.StringValue(c.X)` etc. |
| pointer (`*string`) | create: pointer when known; update: `updateStringPtr(plan, state)`, nil when unchanged | `stringPtrValue` |
| `nullable.Nullable[T]` | phase 1's `createX` / `updateX` | phase 1's `xValue` |
| named string (enum), value or pointer | `setEnum` / `setEnumPtr` / `updateEnumPtr`, generic over `T ~string` and inferred from the destination, so generated code never names api-go's internal types | `types.StringValue(string(c.X))` |
| `time.Time` | (read-only) | RFC 3339 in UTC |
| period custom type | the helpers above on the embedded `StringValue` | `customtypes.NewPeriodValue` / `periodValue` |

Any other shape is a generation error naming the resource and field.

Generated per resource:

- `func (m <model>) createParams() apiv3.<CreateParams>`: every writable
  attribute. Null and unknown values are left out.
- `func (m <model>) updateParams(state <model>) apiv3.<UpdateParams>`: only
  changed values. A value removed from config is sent as null, a default is
  sent when it differs from state, and an unknown is left out (spec 5.3).
- `func (m *<model>) fromAPI(c *apiv3.<Model>)`: every attribute the resource
  keeps, including path parameters read back (`project_id`).

Path parameters are never in the params structs. The wrapper passes them.

### 3.3 Generated wrappers

Path parameters come from the operations: create's are the parent
(`project_id`), and read's last one is the object's own id. The generator
checks with `go/types` that `client.<Service>.Create/Get/Update/Delete` exist
and take those parameters in that order, plus the params type. A missing
method or a mismatched signature is a generation error.

`<name>_resource_gen.go` gets the resource type and its constructor, plus:

- `Metadata` (`honeybadger_<name>`), `Schema` and `Configure`, which takes
  the `*apiv3.Client` from provider data. `Configure` returns early when
  provider data is nil, which happens during validation before the provider
  is configured;
- **Create:** `createParams`, the client call, then `fromAPI`. If
  `isUnknownOutcome`, it shows the phase 1 "may have been created" error,
  with the import format and `find_hint`. A 2xx whose result has an empty
  `id` (api-go accepts `{"data":{}}`) gets the same error instead of saving
  an empty state (umbrella 6.4);
- **Read:** a 404 removes the resource from state (spec 5.3);
- **Update:** `updateParams(state)`, the client call, then `fromAPI`;
- **Delete:** a 404 counts as success; an unknown outcome gets the "delete
  may still be in progress" error;
- **ImportState:** parses the generated import format into its attributes;
- `ConfigValidators`, when `config_validators` names a function.

The generator owns `resources_gen.go`, which defines
`generatedResources() []func() resource.Resource`. `provider.go`'s
`Resources` is edited by hand once, to return `generatedResources()` plus any
`crud: manual` resources. A `crud: manual` resource gets only its schema,
model and mapping, and its hand-written wrapper is added to that list.

## 4. The resources

```yaml
project:
  create: createProject
  read: getProject
  update: updateProject
  delete: deleteProject
  import: "<id>"
  service: Projects
  api: {model: Project, create_params: ProjectCreateParams, update_params: ProjectParams}
  find_hint: "your Honeybadger projects"
  omit:
    links: web UI links, not configuration
    team_id: team membership belongs to the teams resource
    disable_email_notifications: create-only flag, never stored
    fault_count: live data
    unresolved_fault_count: live data
    last_notice_at: live data
    earliest_notice_at: live data
    environments: live data
    active: live data
  server_filled: [purge_days]
  use_state_for_unknown: [account_id, created_at]

project_key:
  create: createProjectKey
  read: getProjectKey
  update: updateProjectKey
  delete: deleteProjectKey
  import: "<project_id>/<id>"
  service: ProjectKeys
  api: {model: ProjectKey, create_params: ProjectKeyParams, update_params: ProjectKeyParams}
  find_hint: "the project's API keys in Honeybadger"
  use_state_for_unknown: [created_at, key]

check_in:
  create: createCheckIn
  read: getCheckIn
  update: updateCheckIn
  delete: deleteCheckIn
  import: "<project_id>/<id>"
  service: CheckIns
  api: {model: CheckIn, create_params: CheckInCreateParams, update_params: CheckInUpdateParams}
  find_hint: "the project's check-ins in Honeybadger"
  config_validators: checkInConfigValidators
  omit:
    links: web UI links, not configuration
    state: live data
    reported_at: live data
    expected_at: live data
    missed_count: live data
  custom_types: {report_period: period, grace_period: period}
  use_state_for_unknown: [created_at, report_url]
```

Notes:

- **`token` has no `UseStateForUnknown`.** It's the project's newest key, so
  it changes whenever a `honeybadger_project_key` is created or deleted.
  Keeping its prior value at plan time would make a project update applied
  alongside a new key fail with "inconsistent result". It shows "(known after
  apply)" on project updates instead.
- **`report_url` is stable:** it's built from the check-in's hashid, not its
  slug.
- **`slug` is plain Optional.** The server doesn't generate it, so an
  omitted slug stays null and removing one clears it. honeybadger has
  corrected the spec's wording.
- **`cron_timezone` is always read back.** honeybadger fixed the API to
  return `UTC` for simple check-ins and lists the field as required in the
  read schema (bundle `2c4aa8fc…`). 2b copies that bundle in before the
  check-in resource is generated.
- **From the spec, with no override:**
  - defaults: `schedule_type` is `simple`, `cron_timezone` is `UTC`,
    `grace_period` is `00:00:00`, `disable_public_links` is false and
    `resolve_errors_on_deploy` is true;
  - `project_id` forces replacement;
  - enum and pattern validators, and descriptions.

### 4.1 Check-in schedule rules (`checkInConfigValidators`, hand-written)

These mirror the API's rules, which the spec states only in prose:

- an omitted `schedule_type` counts as `simple`. Config validators see the
  raw config, before schema defaults apply;
- `simple` requires `report_period` and forbids `cron_schedule`;
- `cron` requires `cron_schedule` and forbids `report_period`;
- a `cron_timezone` other than `UTC` requires `cron`;
- `report_period` can't be zero (checked with `ParsePeriod`).

Only the API can check that `cron_timezone` is an ActiveSupport zone name,
that `name` is unique within the project, that `slug` matches
`[a-z0-9\-_]{1,255}`, and that the account's plan allows cron check-ins
(403 `feature_unavailable`). Those surface as apply-time diagnostics.

With these rules, the API never changes a field the plan didn't predict when
the schedule switches (umbrella 5.3), so no plan modification is needed.
Values unknown at plan time skip the rules; the API's 422 catches them at
apply.

### 4.2 Shared helpers (hand-written, `internal/provider`)

These go next to phase 1's `nullable.go`, each with plain Go tests:

- `updateStringPtr(plan, state) *string`: nil when unchanged;
- `stringPtrValue(*string) types.String`;
- `setEnum[T ~string](dst *T, v types.String)`,
  `setEnumPtr[T ~string](dst **T, v types.String)` and
  `updateEnumPtr[T ~string](dst **T, plan, state types.String)`;
- `periodValue(nullable.Nullable[string]) customtypes.PeriodValue`;
- `timeValue(time.Time) types.String`.

## 5. Testing

### 5.1 Generator (golden, compiled)

The widget fixture grows to cover:
- every type shape in section 3.2;
- a generated wrapper;
- `resources_gen.go`.

Its golden output is compiled, as in 2a. Error cases, each naming the
resource and field or method:
- a model attribute with no matching Go field, on the write side and on the
  read side;
- a missing service method;
- service method parameters in the wrong order;
- a params type that isn't a struct;
- a create-side value field whose attribute isn't Required.

### 5.2 Unit tests

- Helpers: nil versus unchanged, the enum helpers, period and time reads.
- `checkInConfigValidators`: each forbidden combination, and unknown values
  skipping the rules.
- One `resource.UnitTest` per resource against an `httptest` fake, checking
  the exact create and update request bodies end to end. That covers null
  clears, defaults sent, unknown fields left out, and the pointer, enum and
  period shapes.
- One `resource.UnitTest` showing that a 404 on read removes the resource.
- One `resource.UnitTest` showing that a 404 on delete completes without an
  error.
- One `resource.UnitTest` showing that an unknown-outcome create shows the
  import hint, and one where the create returns `{"data":{}}`.
- One `resource.UnitTest` where the fake answers a create with a 429 and
  `Retry-After: 0`, then succeeds: the body is resent and the apply
  succeeds.
- The delete-timeout diagnostic is tested directly, as in phase 1; it isn't
  driven through a 30-second stall.

### 5.3 Acceptance tests (`TF_ACC`, local stack)

Setup is documented in `CONTRIBUTING.md`:
- `HONEYBADGER_API_URL=http://localhost:3001`;
- `HONEYBADGER_API_TOKEN` set to Kevin's token at run time; it's never
  stored;
- the token's account `api_hits_per_hour` raised well above 360 before a run,
  because a full run makes about 150–250 requests.

`testAccPreCheck` checks both variables, calls `Tokens.Get`, then reads
`client.LastRateLimit().Remaining` and fails early with a clear message if
fewer than 300 requests remain.

Each resource's lifecycle:
1. create;
2. update every mutable attribute;
3. remove each Optional attribute;
4. import;
5. destroy;
6. an object deleted outside Terraform is dropped from state.

The testing framework already fails any step whose plan after apply or
refresh isn't empty, so there are no separate "no changes" steps.

Plan assertions (`ConfigPlanChecks.PreApply`):
- an in-place `Update` for every mutable attribute change;
- `Replace` for a `project_id` change on a key and a check-in;
- for check-in simple → cron, `report_period` null and `cron_timezone` `UTC`.

Specific cases:
- **Period round trips:**
  - `grace_period = "1 week"` (the API stores "7 days") plans no change;
  - changing the configured "01:30:00" to "1 hour 30 minutes" plans one update
    (semantic equality covers values the API returns, not two configured
    forms), and the plan after it is empty;
  - importing a check-in stored as "7 days", with "1 week" in config, plans
    one harmless update and then an empty plan. Import has no prior value
    for semantic equality to keep, so that one update is expected and
    documented.
- **Defaults when an attribute is removed:** create a project with
  `disable_public_links = true`, then drop it from config. The next step's
  `ConfigPlanChecks.PreApply` asserts `ResourceActionUpdate` and
  `disable_public_links` known as `false` (umbrella 5.2). An import step
  can't show this, because import verification compares state with state,
  not with config.
- **Concurrency:** five check-ins with `count = 5` succeed. (Retries on 429
  are proven in the unit tests.)
- **Project and new key in one apply:** `token` shows "(known after apply)"
  and the apply succeeds.
- **Adding a `slug`** to an existing check-in plans an update.
- **Cascade:** destroying a project together with its keys and check-ins
  succeeds.
- **Import IDs:** built with `ImportStateIdFunc` (`<project_id>/<id>`).

A `tf-acc-` sweeper lists projects, filters by prefix and deletes them. Each
project delete cascades to its keys and check-ins.

## 6. Out of scope

- Data sources (deferred, umbrella D5).
- Array item enums and patterns, nested objects, and arrays of objects in
  the generator (phase 4, if needed).
- A dev-only rake task that issues tokens (honeybadger's to build; acceptance
  uses Kevin's token until then).
