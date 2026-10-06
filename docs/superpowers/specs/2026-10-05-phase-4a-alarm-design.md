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
code. The generator learns what the alarm needs today: nested objects whose
attributes are all required, collections of plain values, and resource
descriptions. The resource passes a full acceptance lifecycle against Kevin's
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
come from the object's own properties, with their enums, patterns, bounds and
descriptions. Nesting recurses (`trigger_config.config`).

Every attribute inside an object must be required, as every key of
`AlarmTriggerConfig` and `AlarmTriggerCondition` is today. An optional or
defaulted key, a read-only or write-only one, or an exclusive bound inside an
object is a generation error ("isn't supported yet; extend specgen"). Those
need their own rules, because the object is replaced whole, and they're
added against the real contract when the bundle first needs them.
`server_filled` can't name a nested attribute.

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
Every child is required, so at apply time every child is known. If one
isn't, `toAPI` returns an error diagnostic naming the
attribute instead of sending a zero value.

A nested object's api-go field may be a value struct, a pointer, or
`nullable.Nullable[Struct]`. Value on create needs a Required attribute, as
for scalars in 2b. Any other shape is a generation error naming the
resource and field.

### 3.2 Collections of scalars

Arrays of strings, integers, numbers and booleans become list attributes, or
set attributes when `uniqueItems` is true on any of the three schemas (as
`setKind` decides today). `minItems` and `maxItems` become size validators
(`SizeAtLeast`, `SizeAtMost`); today they're a generation error
(`merge.go:204`).

Go shapes, extending the 2b shape table:

| Go shape | Write | Read |
| --- | --- | --- |
| `[]T` | create, Required only | list or set of `T` |
| `*[]T` | create: pointer when known; update: pointer when changed | list or set, null when nil |
| `nullable.Nullable[[]T]` | create: set when known; update: set when changed, null when removed | list or set, null when unset or null |

As for scalar pointers in 2b, a `*[]T` update field can't send null, so it's
a generation error for an attribute the user can remove, unless the
attribute is server-filled (removal keeps the stored value).

Item enums and patterns, and arrays of objects, stay generation errors until
phase 4b needs them (integration `events` and `filters`).

### 3.3 Resource descriptions

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
| `evaluation_period` | Required, pattern `^\d+[smhd]$` | create input |
| `lookback_lag` | Required, pattern `^\d+[smhdw]$` | create input |
| `trigger_config` | Required nested: `type` (enum `alert_result_count`), `config` (nested: `operator` enum, `value` number) | `AlarmTriggerConfig` |
| `stream_ids` | Optional + Computed set, at least one element | server-filled |
| `id`, `created_at` | Computed | read model |

`created_at` is required and non-null on the `Alarm` read schema from bundle
`502ca13c`, so api-go generates `time.Time`. The provider doesn't add a
pointer-time shape.

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

The spec states some rules only in prose: `value` must be a whole number of
at least 0 for `alert_result_count`. The provider doesn't mirror rules that
depend on the trigger type. The API's 422 catches them at apply, and field errors map to the attribute
(`trigger_config.config.value`, for example) through `fieldPath`. Only
what each key declares on its own is checked at plan time.

### 4.3 Known API behaviour

Documented in the resource docs, not worked around (umbrella 10):

- **An invalid trigger on update answers 200 and keeps the old one.** The
  read-back then differs from the plan, and Terraform reports "Provider produced
  inconsistent result after apply", which surfaces the problem.
- **A create that times out can leave an Opticon observer behind** with no
  alarm. The unknown-outcome error's `find_hint` sends the user to the
  project's alarms.
- **A retried delete can fail** if Opticon already deleted its observer.
- **An alarm stored with no trigger** reads as null into a Required
  attribute, so importing it plans a change that adds one. `trigger_config`
  stays nullable on read (Kevin): Opticon always has a trigger, but v3 reads
  Honeybadger's stored copy of the observer, and an old copy can lack it.
  The resource docs say so under import.
- **Opticon unavailable answers 503.** On create that's an unknown outcome
  (umbrella 6.4); on read, update and delete it's an error.

## 5. Testing

### 5.1 Generator (golden, compiled)

The widget fixture gains a nested object and collections covering every
case in section 3:

- a Required nested object containing a nested object (two levels), whose
  attributes are all required: an enum, an integer and a number;
- a set of strings with `minItems`, mapped from `*[]string` on one input and
  `nullable.Nullable[[]string]` on the other;
- a resource-level description.

Golden output is compiled, as in 2a and 2b, and the fixture includes the
read shapes explicitly (`Nullable[Struct]`, `Nullable[[]string]`). Error
cases, each naming the resource and field: an array of objects, an item
enum, an optional or defaulted key inside an object, an exclusive bound, a
nested object whose api-go field has an unsupported shape, a `*[]T` update
field on a removable attribute that isn't server-filled, and `server_filled`
naming a nested attribute.

### 5.2 Unit tests

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
- **honeybadger, done after the spec review** (03a7e54bf5, bundle
  `502ca13c`): `Alarm` lists every always-returned key as required,
  `created_at` and `updated_at` included; `evaluation_period` is
  `^\d+[smhd]$` and `lookback_lag` `^\d+[smhdw]$` on both inputs; a
  description for `AlarmCreateInput.name`. `trigger_config` stays nullable on
  read (section 4.3).
- **Bundle:** 4a starts by copying honeybadger's bundle `502ca13c` or later.
- **api-go:** regenerated from the same bundle by mcp-v3, so
  `make check-spec` matches. The generator accepts either update shape for
  `stream_ids` (`*[]string` or `Nullable[[]string]`).

## 7. Out of scope

- Phase 4b: integrations, arrays of objects, the variants expansion.
- Alarm history (`ListHistory`): live data, a future data source.
- The `honeybadger_streams` data source (deferred, umbrella D5).
- Plan-time trigger rules that depend on `type` (section 4.2).
- Trigger settings Opticon hasn't shipped, and the generator support they'd
  need (optional and defaulted keys inside objects, exclusive bounds): added
  when they're in the bundle.
