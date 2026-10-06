# Phase 6: Dashboards: Design

Date: 2026-10-06
Status: Draft for review
Parent: `2026-10-05-terraform-provider-design.md` (the umbrella spec). This
spec covers phase 6: `honeybadger_dashboard`. It builds on the generator as
phase 5 left it (`2026-10-06-phase-5-design.md`).

## 1. Goal

Ship `honeybadger_dashboard`, generated from honeybadger's bundle and api-go's
`apiv3` types with one `resources.yaml` entry. Widgets are one JSON document
(Kevin's decision): each widget's `config` has a different shape per widget
type, and typing them all in Terraform would track every widget change in
Honeybadger for little gain.

The standing rules hold: the generator contains as little Honeybadger domain
logic as possible; schema problems are fixed in the spec first; local
acceptance is the gate.

## 2. Decisions

| Decision | Choice |
|---|---|
| Widget shape | `widgets` is a JSON string, written with `jsonencode([...])`. |
| Comparison | A generic JSON subset rule (section 3.1), honeybadger's recommendation (option A), agreed by Kevin. Not strict equality (every server-filled key would have to be written), not a list of known server fields (domain logic). |
| Widget ids | Required in Terraform, not in the API (Kevin). The API keeps ids optional (the web UI's YAML editor assigns them and shows them back); Terraform can't write generated ids back into config, so a widget without one would get a new id on every update and break its links. `resources.yaml` lists `id` as a key each widget must set (section 3.1), checked at plan time. Ids are kept as sent and unique within their dashboard; another dashboard can use the same id. Docs advise letters, digits, `-` and `_` (the id appears in web UI links). Duplicates are left to the API's 422. |
| Layout | The config is the source of truth (Kevin). Honeybadger merges a written `grid` into the default `{x: 0, y: 0, w: 12, h: 3}`: left out, the default; partial (`{x: 2}`), the missing keys from the default. The subset rule needs no grid logic: keys the user set match, keys Honeybadger filled are ignored. A widget moved in the web UI is put back by the next apply that writes the dashboard. Docs: set `grid` to control layout. |
| Default dashboard | Not special. Honeybadger creates a project's default dashboard (`is_default`) the first time someone opens Insights, and v3 doesn't protect it. Users adopt it with import. A Terraform destroy deletes it, and the next Insights visit creates a new one outside Terraform. |
| `updated_at` | Omitted: it changes on every save (as the alarm's `updated_at`). |

## 3. Generator extensions

### 3.1 The `json_subset` custom type

A custom type in `internal/provider/customtypes`, beside `period`:

- **Validation** (`ValidateAttribute`, skipped while the value is unknown):
  the value is JSON of the schema's own top-level type (an array, for
  `widgets`, from the bundle's `type: array`); and, when the field has
  required keys (`json_required_keys` in `resources.yaml`), every array
  element is an object that sets each listed key to a value that isn't
  empty. Errors name the element: `widgets[2]: "id" is required`.
- **Semantic equality** (`StringSemanticEquals`). The framework calls it on
  the value just read, with the planned or prior value as the argument
  (framework `value_semantic_equality_string.go`): after create and update
  against the plan, after read against prior state; never while planning;
  not on import, whose prior value is null. They're equal when the argument
  (what the user wrote) is a subset of the receiver (what Honeybadger
  returned):
  - "Not set" means `null`, `""`, `[]` or `{}`, or an object that is empty
    once those are removed: exactly what Honeybadger drops on read. A key
    whose planned value is not set matches anything, including a missing key
    (`streams = []` matches the every-stream list a read returns).
  - Objects: every key the user set matches the read value's; keys only in
    the read value are ignored.
  - Arrays containing any object or array: same length, compared position by
    position (Honeybadger keeps widgets in the order written).
  - Arrays of scalars only (strings, numbers, booleans): compared as
    multisets, ignoring order but counting duplicates (Honeybadger stores
    `streams` in database order).
  - Scalars: equal values; numbers compare numerically (`3` equals `3.0`).
    `false` and `0` are values, not "not set".

  When they're equal the framework keeps the argument, so state holds the
  user's JSON as written, and the next plan compares config with it
  directly. A real change (a value the user set, changed elsewhere; a widget
  added or removed; a value the server refused or dropped) breaks the
  subset: a read puts Honeybadger's JSON in state and the plan shows the
  whole string changing back, or the apply fails with "Provider produced
  inconsistent result after apply".

  The multiset rule is safe because, today, the only scalar array in any
  widget config is `streams`, which is a set. A future order-significant
  scalar array would need the spec to say so; that's recorded for
  honeybadger (section 6).

### 3.2 Merge and mapping

- **Merge:** a field named in `custom_types` as `json_subset` is decided
  before `setKind`, which would otherwise refuse it (an object whose
  `config` has no properties). It becomes a string attribute whatever its
  JSON type; Merge records the schema's top-level JSON type for validation.
  A declared default is JSON-encoded (`default: []` → `"[]"`, `{}` → `"{}"`).
- **`json_required_keys`:** a new `resources.yaml` key, `{field: [keys]}`,
  allowed only on `json_subset` fields. It's per-resource configuration, like
  `omit`, so the generator holds no dashboard knowledge.
- **Create** decodes the JSON strictly (`DisallowUnknownFields`) into the API
  field, whatever its Go shape (here `*[]DashboardWidgetInput`), so a
  misspelt top-level widget key (`presentaton`) is an error naming it, and no
  request is sent. `config` is a free map, so a typo there reaches the
  server, whose dashboard validator answers 422.
- **Update** sends the field only when the plan differs from state.
- **Read** encodes the API field back to JSON.

## 4. The resource

`honeybadger_dashboard`, project-scoped: `project_id`, `title` (required),
`default_ts` (optional, nullable), `widgets` (`json_subset`, default `[]`,
every widget needs an `id`), and `is_default`, `shared`, `links.web`,
`created_at` (read-only).

```yaml
dashboard:
  create: createDashboard
  read: getDashboard
  update: updateDashboard
  delete: deleteDashboard
  import: "<project_id>/<id>"
  service: Dashboards
  api: {model: Dashboard, create_params: DashboardCreateParams, update_params: DashboardUpdateParams}
  find_hint: "the project's dashboards in Honeybadger"
  custom_types: {widgets: json_subset}
  json_required_keys: {widgets: [id]}
  omit:
    updated_at: live data
  use_state_for_unknown: [created_at, links, is_default, shared]
```

Docs (hand-written template) cover:
- every widget needs an `id`, kept as written (letters, digits, `-`, `_`);
- set `grid` to control layout: left out it's `{x: 0, y: 0, w: 12, h: 3}`, a
  partial grid fills from that, and moves made in the web UI are put back by
  the next apply that writes the dashboard;
- `streams` left out or `[]` means every stream;
- "inconsistent result after apply" means Honeybadger didn't keep a value
  you set; the dashboard is in state, so fix the config (or leave the key
  out) and apply again;
- drift shows as the whole `widgets` string changing;
- reformatting the JSON (key order, a heredoc instead of `jsonencode`) plans
  an update that changes nothing;
- import: include the dashboard's `widgets` in config before applying (with
  `widgets` left out, the default `[]` would remove every widget); the first
  plan after import is an update that settles state to your JSON;
- the default dashboard can be imported, and Honeybadger recreates one after
  a destroy.

The example sets ids and grids, and interpolates a resource attribute into a
widget's `presentation.title`.

Known trade-off: a key someone adds in the web UI that the user's JSON never
mentions isn't shown as drift.

## 5. Testing

### 5.1 The custom type

Table tests for semantic equality, each run in the framework's direction and
reversed (asymmetric cases must differ): server-only keys; every "not set"
form at every level, including an object empty once its empty keys go;
`streams: []` against a full list; arrays of objects by position (a reorder
is unequal); scalar arrays as multisets (order ignored, `["a","a"]` vs
`["a"]` unequal); `3` vs `3.0`; `false` and `0` kept; a changed value, a
missing widget, an extra widget, `"3"` vs `3`.

Validation tests: invalid JSON; the wrong top-level type (an object, or
`null`, for `widgets`); a widget without `id`, or with `id = ""`; an element
that isn't an object; an unknown value skipped.

### 5.2 Generator

A `json_subset` field on the sprocket fixture (an array of structs, with
`default: []` and a required key): golden and compiled. Merge decides it
before `setKind` and leaves it a string; the default renders `"[]"` (and a
`{}` default renders `"{}"`); `json_required_keys` on a field that isn't
`json_subset` is refused; mapping uses the strict decode on create, the
changed-only send on update, and the encode on read.

### 5.3 Unit tests (fake API)

The fake API stores widgets and, on read, does what honeybadger does: merges
`grid` into the default, lists every stream for an `insights_vis` widget
whose `streams` is left out or empty, and drops `null`, `""`, `[]` and `{}`.
Cases:

- create, then an empty plan (no grid, partial grid, `streams = []`);
- a value the user set, changed outside Terraform: the plan shows it;
- a widget added, reordered, removed: each an update with the whole list;
- `widgets` removed from config: sends `[]`;
- a widget without `id`: a plan-time error, no request sent;
- a misspelt top-level widget key: an error naming it, no request sent;
- an unknown stream slug the fake drops: "inconsistent result" on apply;
- import with the widgets in config: one update that settles; import with
  `widgets` left out: the plan sends `[]` (pinned, and documented);
- a heredoc with reordered keys against `jsonencode` state: a no-op update.

### 5.4 Acceptance (local stack)

A dashboard with ids, an `insights_vis` widget without `streams` and with a
partial grid, and an `alarms` widget whose `presentation.title` interpolates
a `honeybadger_project` attribute (unknown at plan, settled on the first
apply); add a widget; reorder; remove one; import with the widgets in config
(`ImportStateVerifyIgnore: ["widgets"]`, then a plan that settles it).

## 6. Dependencies

- **honeybadger** (pending batch, bundle `41046164`, uncommitted until Kevin
  okays it): `DashboardWidgetInput.id` kept as sent, unique per dashboard,
  with the character advice; `grid` merged into its default, documented;
  widgets read in the order written; the read drops exactly `null`, `""`,
  `[]` and `{}`; `insights_vis.streams` corrected (left out or empty means
  every stream); `widgets` `default: []` on both inputs; deleteDashboard
  describes the default dashboard. Recorded for honeybadger: the provider
  compares scalar arrays inside widgets as multisets, so an
  order-significant scalar array added later needs the spec to say so.
- **api-go** (mcp-v3): `Dashboards` exists (Get, Create, Update, Delete);
  re-vendor after honeybadger commits.

## 7. Out of scope

- Typed widgets, per-widget resources, and validating `config` against its
  per-type schema in the provider (the server validates on apply).
- A read-only copy of the stored widgets (`stored_widgets`): required ids
  make it unnecessary.
- Listing dashboards (data sources stay deferred, D5).
