# Phase 4a: Alarm Resource: Design

Date: 2026-10-05
Status: Draft for review
Parent: `2026-10-05-terraform-provider-design.md` (the umbrella spec). This
spec covers phase 4a only: `honeybadger_alarm`. Phase 4b
(`honeybadger_integration_*`) gets its own spec.

## 1. Goal

Ship `honeybadger_alarm`, generated the same way as the phase 2b resources:
schema, model, mapping and CRUD wrapper come from the bundle and api-go's
`apiv3` types, with one `resources.yaml` entry and no hand-written resource
code. The generator learns nested objects and collections, which phase 4b
also needs. The resource passes a full acceptance lifecycle against Kevin's
local stack, Opticon included.

## 2. Decisions

| Decision | Choice |
| --- | --- |
| Phase 4 split | 4a is the alarm, 4b the integrations, each with its own spec, plan and review |
| `trigger_config` | Generated nested object; no hand-written validation |
| Rules that depend on the trigger type | Left to the API's 422s (section 4.2) |
| Live data (`state`, `error`, `last_checked_at`, `next_check_at`, `updated_at`) | Omitted (umbrella D5) |
| `stream_ids` | Server-filled (option B): omitted on create, the API stores the current streams; removing it from config keeps the stored list |
| `honeybadger_streams` data source (umbrella D5) | Deferred; `stream_ids` takes raw ids |
| Durations (`evaluation_period`, `lookback_lag`) | Plain strings; the API returns them exactly as sent |

## 3. Generator extensions

### 3.1 Nested objects

A property whose schema is an object (inline, or a `$ref` to one, or a
nullable `anyOf` of one) becomes a `SingleNestedAttribute`. Its attributes
come from the object's own properties by the same rules as top-level ones
for Required, defaults, enums, patterns, bounds and descriptions. Nesting
recurses (`trigger_config.config`).

One rule differs: an optional child without a declared default is plain
Optional. It's never Computed, `UseStateForUnknown` or server-filled, and
`server_filled` can't name it. Because the object is replaced whole, a child
kept from state would be resent after the user removed it: switching
`trigger_config.type` back from `alert_field_threshold` would resend
`field`. A removed child is simply absent from the object sent.

Generated per nested object:

- a model struct with `tfsdk` tags, held in the parent model as
  `types.Object`, so null and unknown survive;
- its attribute-type map, for building and reading the `types.Object`;
- `toAPI` and `fromAPI` functions for the api-go struct it maps to, matched
  by JSON tag and Go shape as in 2b (section 3.2 of the 2b spec). The api-go
  struct is found from the parent field's Go type, so no type name is
  configured.

Writes treat a nested object as one value, because the API replaces it
whole:

- **create:** the whole object;
- **update:** the whole object when the planned object differs from state,
  and nothing when it doesn't. Changing one key inside sends every key;
- **read:** a null object reads as null; otherwise every key is read back.

`toAPI` requires the object and every value inside it to be known. A
`types.Object` can be known while a child is unknown, and reading an unknown
number gives 0, so an unchecked conversion could send a threshold of 0.
With the rule above, no child is Computed, so at apply time every child is
known. If one isn't, `toAPI` returns an error diagnostic naming the
attribute instead of sending a zero value.

A nested object's api-go field may be a value struct, a pointer, or
`nullable.Nullable[Struct]`. Value on create needs a Required attribute, as
for scalars in 2b. Any other shape is a generation error naming the
resource and field.

### 3.2 Collections of scalars

Arrays of strings, integers, numbers and booleans become list attributes, or
set attributes when `uniqueItems` is true on any of the three schemas (as
`setKind` decides today). Item enums and patterns become element validators.
`minItems` and `maxItems` become size validators (`SizeAtLeast`,
`SizeAtMost`); today they're a generation error (`merge.go:204`).

Go shapes, extending the 2b shape table:

| Go shape | Write | Read |
| --- | --- | --- |
| `[]T` | create, Required only | list or set of `T` |
| `*[]T` | create: pointer when known; update: pointer when changed | list or set, null when nil |

As for scalar pointers in 2b, a `*[]T` update field can't send null, so it's
a generation error for an attribute the user can remove, unless the
attribute is server-filled (removal keeps the stored value).
| `nullable.Nullable[[]T]` | create: set when known; update: set when changed, null when removed | list or set, null when unset or null |
| `[]E`, `*[]E`, `Nullable[[]E]` with `E ~string` (enum items) | as above, converted element by element | as above |

Arrays of objects stay a generation error until phase 4b needs them
(integration `filters`).

### 3.3 Defaults and bounds

Defaults inside nested objects use the same rules as top-level attributes:
a declared `default` makes the attribute Optional and Computed with a
static default. The generator also reads `exclusiveMinimum` and
`exclusiveMaximum` (OpenAPI 3.1 numeric form) into validators. The framework
has no exclusive-bound validators, so a small generic one goes in
`internal/provider` with the other shared helpers.

This is what keeps the planned Opticon changes generation-only. When
honeybadger documents `alert_field_threshold`, anomaly mode and hysteresis,
each new `trigger_config` key arrives with its default and bounds
(`on_no_data: keep_state`, `detection_mode: static`, `alert_after` and
`resolve_after` default 1 with minimum 1, `sigma` with `exclusiveMinimum: 0`),
as honeybadger has agreed. Regenerating picks them up. The fixture in
section 5.1 proves that before honeybadger ships them.

### 3.4 Resource descriptions

The read model's schema `description` becomes the resource's
`MarkdownDescription`, so it reaches the registry docs. That closes a
deferred 2b minor (`render.go:181`); `ProjectKey` already has one.

## 4. The resource

```yaml
alarm:
  create: createAlarm
  read: getAlarm
  update: updateAlarm
  delete: deleteAlarm
  import: "<project_id>/<id>"
  service: Alarms
  api: {model: Alarm, create_params: AlarmCreateParams, update_params: AlarmUpdateParams}
  find_hint: "the project's alarms in Honeybadger"
  omit:
    links: web UI links, not configuration
    state: live data; lags its evaluation history (umbrella 7)
    error: live data
    last_checked_at: live data
    next_check_at: live data
    updated_at: live data
  server_filled: [stream_ids]
  use_state_for_unknown: [created_at]
```

The resulting schema:

| Attribute | Kind | From |
| --- | --- | --- |
| `project_id` | Required, forces replacement | path parameter |
| `name`, `query` | Required | create input |
| `description` | Optional; removing it sends null | nullable on update |
| `evaluation_period`, `lookback_lag` | Required, pattern `^\d+[smhdw]$` | create input; update pattern |
| `trigger_config` | Required nested: `type` (enum `alert_result_count`), `config` (nested: `operator` enum, `value` number) | `AlarmTriggerConfig` |
| `stream_ids` | Optional + Computed set, at least one element | server-filled |
| `id`, `created_at` | Computed | read model |

`created_at` needs honeybadger to list it as required on the `Alarm` read
schema. Today it's optional there, so api-go generates `*time.Time`, which
the generator doesn't map. honeybadger has the request (section 6). The
provider doesn't add a pointer-time shape for a field that's always
present.

### 4.1 `stream_ids`

An alarm always has an explicit list of streams. Leaving `stream_ids` out of
a create is a shortcut for "the streams that exist now", not an "all
streams" setting (Kevin). So:

- set in config: Terraform manages exactly that set;
- left out: the API stores the project's current streams, and state shows
  them;
- removed from config: Terraform stops managing it, and the stored list
  stays. The docs say so: to change the streams, list them.

`[]` is refused at plan time by the size validator from `minItems: 1`. The
API's own 422 (`can't be empty; send null for every stream`) maps to the
attribute if it's reached.

**Streams are provisioned asynchronously** after a project is created (a
Sidekiq job creates `default`, then `internal`). An alarm created in the same
apply, with `stream_ids` left out, can today get a 422 (no streams yet) or
silently store only `default`. honeybadger is changing the alarm create to
answer 429 with `Retry-After` and its own code until both streams exist,
storing nothing. api-go already retries a POST on 429 and the provider
already treats 429 as nothing stored, so the provider needs no change.

The retry budget is about three times `Retry-After`: the provider's policy
(`MaxAttempts: 4`) waits three times, each for exactly `Retry-After`
(honeybadger sends 2 seconds), so about 6 seconds. `MaxWait` caps one wait,
not the total. That covers normal provisioning (about a second). Under a
Sidekiq backlog, the apply fails with honeybadger's `streams_provisioning`
message (shown through `addAPIError`'s default branch) and succeeds on
rerun. The policy stays as it is.

### 4.2 Trigger rules

The spec states some rules only in prose: today `value` must be a whole
number of at least 0 for `alert_result_count`; after Opticon's observer-26
branches ship, `field` is required for `alert_field_threshold`, and
anomaly mode needs `direction` and `sigma` but not `operator` or `value`.
Those rules are still changing, so the provider doesn't mirror them. The
API's 422 catches them at apply, and field errors map to the attribute
(`trigger_config.config.value`, for example) through `fieldPath`. Only
what each key declares on its own is checked at plan time.

### 4.3 Known API behaviour

Documented in the resource docs, not worked around (umbrella 10):

- **An invalid trigger on update answers 200 and keeps the old one**, until
  Opticon's `observer-26-field-threshold` branch lands. The read-back then
  differs from the plan, and Terraform reports "Provider produced
  inconsistent result after apply", which surfaces the problem.
- **A create that times out can leave an Opticon observer behind** with no
  alarm. The unknown-outcome error's `find_hint` sends the user to the
  project's alarms.
- **A retried delete can fail** if Opticon already deleted its observer.
- **An alarm stored with no trigger** (`trigger_config` is nullable on read)
  reads as null into a Required attribute, so its next plan adds one. That's
  the intended way it surfaces. honeybadger is asked whether v3 alarms can
  have no trigger at all (section 6); if not, the read schema becomes
  non-null and the case disappears.
- **New defaulted trigger keys after an upgrade** (Opticon's hysteresis
  keys, for example) plan one harmless update on existing alarms the first
  time, as the default fills in.
- **Opticon unavailable answers 503.** On create that's an unknown outcome
  (umbrella 6.4); on read, update and delete it's an error.

## 5. Testing

### 5.1 Generator (golden, compiled)

The widget fixture gains a nested object and collections covering every
case in section 3:

- a Required nested object containing a nested object (two levels), with an
  enum, an integer and a number, an Optional key with a declared default,
  and a key with `exclusiveMinimum`;
- a set of strings with `minItems`, mapped from `*[]string` on one input and
  `nullable.Nullable[[]string]` on the other;
- a list of enum strings;
- a resource-level description.

Golden output is compiled, as in 2a and 2b, and the fixture includes the
read shapes explicitly (`Nullable[Struct]`, `Nullable[[]string]`). Error
cases, each naming the resource and field: an array of objects, a nested
object whose api-go field has an unsupported shape, a value-shaped nested
field on create whose attribute isn't Required, a `*[]T` update field on a
removable attribute that isn't server-filled, and `server_filled` naming a
nested child.

Compiling doesn't prove runtime behaviour, so the rendered fixture also runs
in a `resource.UnitTest` against a fake: a nested default fills in at plan
time, item validators and the size validator reject bad values, a list and a
set round-trip without a diff, and removing an optional nested child leaves
it out of the next body.

### 5.2 Unit tests

- The exclusive-bound validator: at, below and above the bound; null and
  unknown skip it.
- One `resource.UnitTest` for the alarm against the `httptest` fake,
  checking exact request bodies:
  - create sends the whole `trigger_config` and leaves `stream_ids` out;
  - changing only `trigger_config.config.value` sends the whole
    `trigger_config` and nothing else;
  - removing `description` sends `"description": null`;
  - removing `stream_ids` from config plans no change and sends nothing.
- `stream_ids = []` fails at plan time.
- An alarm create answered with 429 and the `streams_provisioning` code
  (`Retry-After: 0`), then 201: the body is resent once and the apply
  succeeds.
- A create with `stream_ids` listed sends exactly that list.
- `toAPI` on a known `trigger_config` holding an unknown `value` returns the
  error diagnostic and sends nothing.
- A 422 with field `trigger_config.config.value` maps to that attribute.
- An update answered 200 with the old trigger (the Opticon bug in 4.3)
  fails with "inconsistent result after apply".

### 5.3 Acceptance tests (`TF_ACC`, local stack with Opticon and Sidekiq)

`TestAccAlarm`, about 50 requests:

1. Create an alarm with `stream_ids` left out.
2. Update every mutable attribute: name, query, description, both
   durations, the trigger's operator and value.
3. Narrow `stream_ids` to one stream. A second alarm in the same config
   takes it from the first, since there's no data source:
   `stream_ids = [sort(tolist(honeybadger_alarm.a.stream_ids))[0]]`.
4. Remove `description` (plans null) and `stream_ids` (plans no change).
5. Move the alarm to another project: the plan is a replace.
6. Import with `ImportStateVerify`, then destroy.

`TestAccAlarmWithNewProject`: a project and an alarm in one apply, with
`stream_ids` left out. It asserts the alarm's `stream_ids` has two elements,
which catches a partial snapshot and exercises the `streams_provisioning`
retry.

`TestAccAlarmDeletedOutsideTerraform`: the alarm is deleted through the
client between steps; the next plan re-creates it.

The `tf-acc-` sweeper needs no change: deleting a project deletes its
alarms. `CONTRIBUTING.md` adds that alarm tests need Opticon and Sidekiq
running locally.

## 6. Dependencies

- **honeybadger, done:** the `streams_provisioning` 429 (da5ec7d0aa, bundle
  `189edafa`), `minItems: 1` and a nullable update `stream_ids`.
- **honeybadger, requested from the spec review:**
  - list `created_at` and `updated_at` as required on `Alarm`;
  - a duration `pattern` on both inputs, matching what each field accepts
    (the update pattern allows `w`, which `evaluation_period` refuses);
  - whether a v3 alarm can have no trigger; if not, a non-null
    `trigger_config` on read;
  - a description for `AlarmCreateInput.name`.
- **Bundle:** 4a starts by copying honeybadger's latest bundle, `189edafa`
  or later with the requested fixes.
- **api-go:** regenerated from the same bundle by mcp-v3, so
  `make check-spec` matches. The generator accepts either update shape for
  `stream_ids` (`*[]string` or `Nullable[[]string]`).

## 7. Out of scope

- Phase 4b: integrations, arrays of objects, the variants expansion.
- Alarm history (`ListHistory`): live data, a future data source.
- The `honeybadger_streams` data source (deferred, umbrella D5).
- Plan-time trigger rules that depend on `type` (section 4.2).
- `warning` thresholds (severity levels): not implemented in Opticon yet.
