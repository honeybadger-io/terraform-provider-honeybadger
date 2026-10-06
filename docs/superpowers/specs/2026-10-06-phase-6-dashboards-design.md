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
| Widget ids | Users set their own (`"error-rate"`): honeybadger keeps any string as sent, unique within its dashboard; another dashboard can use the same id. Left out, the server assigns a UUID on every write, which breaks bookmarked widget links. Docs and examples set ids, using letters, digits, `-` and `_` (the id appears in web UI links). No provider warning or validator: that would be a rule about widget `id` in the generator. |
| Default dashboard | Not special. Honeybadger creates a project's default dashboard (`is_default`) the first time someone opens Insights, and v3 doesn't protect it. Users adopt it with import. A Terraform destroy deletes it, and the next Insights visit creates a new one outside Terraform. |
| `updated_at` | Omitted: it changes on every save (as the alarm's `updated_at`). |

## 3. Generator extensions

### 3.1 The `json_subset` custom type

A custom type in `internal/provider/customtypes`, beside `period`:

- **Value:** a string holding JSON. `ValidateAttribute` refuses a value that
  isn't valid JSON, at plan time.
- **Semantic equality** (`StringSemanticEquals`): the framework calls it
  after every create, update and read with the planned or prior value and the
  value the provider read. They're equal when the planned JSON is a subset of
  the read JSON:
  - objects: every key in the planned value matches the read value's; a key
    whose planned value is empty (`null`, `[]`, `{}`) matches a missing key;
    keys only in the read value are ignored;
  - arrays of objects (or mixed): same length, compared position by
    position;
  - arrays of scalars only: compared as sets (honeybadger stores `streams` in
    database order, not the order written);
  - scalars: equal values; numbers compare numerically (`3` equals `3.0`).

  When they're equal the framework keeps the planned or prior value, so state
  holds the user's JSON as written and the next plan compares config with it
  directly. A real change (a value the user set, changed elsewhere; a widget
  added or removed; a value the server refused or dropped) breaks the subset:
  it shows as drift, or fails the apply with "Provider produced inconsistent
  result after apply".

  The scalar-array rule is safe because, today, the only scalar array in any
  widget config is `streams`, which is a set. A future order-significant
  scalar array would need the spec to say so; that's recorded for
  honeybadger (section 6).

### 3.2 Merge and mapping

- `custom_types: {<field>: json_subset}` in `resources.yaml` makes the field
  a string attribute whatever its JSON type, so Merge never types its
  contents. A declared default becomes the default's JSON text
  (`default: []` → `"[]"`).
- Create decodes the JSON strictly (`DisallowUnknownFields`) into the API
  field's Go type, whatever its shape, so a misspelt top-level widget key
  (`presentaton`) is an error naming it. `config` is a free map, so a typo
  there reaches the server, whose dashboard validator answers 422.
- Update sends the field only when the plan differs from state.
- Read encodes the API field back to JSON.

## 4. The resource

`honeybadger_dashboard`, project-scoped: `project_id`, `title` (required),
`default_ts` (optional, nullable), `widgets` (`json_subset`, default `[]`),
and `is_default`, `shared`, `links.web`, `created_at` (read-only).

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
  omit:
    updated_at: live data
  use_state_for_unknown: [created_at, links, is_default, shared]
```

Docs (hand-written template) note: set widget ids; a `grid` left out is the
fixed default `{x: 0, y: 0, w: 12, h: 3}`, so widgets without one overlap;
`streams` left out means every stream; the "inconsistent result" error means
the server didn't keep a value you set; import shows one update, after which
state holds your JSON; the default dashboard can be imported, and is
recreated by Honeybadger after a destroy. The example sets ids and grids, and
an alarms widget references a `honeybadger_alarm`.

Known trade-off: a key someone adds in the web UI that the user's JSON never
mentions isn't shown as drift.

## 5. Testing

### 5.1 The custom type

Table tests for the subset rule: server-only keys; empty vs absent at every
level; arrays of objects by position (and a reorder is unequal); scalar
arrays as sets; `3` vs `3.0`; a changed value, a missing widget, an extra
widget, a type change (`"3"` vs `3`); invalid JSON refused by validation.

### 5.2 Generator

A `json_subset` field on the sprocket fixture (an array of structs, with
`default: []`): golden and compiled; Merge leaves it a string; mapping uses
the strict decode on create, the changed-only send on update, and the encode
on read.

### 5.3 Unit tests (fake API)

The fake API stores widgets and, on read, adds what honeybadger adds: a UUID
for a widget without `id`, the default `grid`, every stream for an
`insights_vis` widget without `streams`, and drops empty values. Cases:

- create, then an empty plan;
- a value the user set, changed outside Terraform: the plan shows it;
- a widget added, reordered, removed: each an update with the whole list;
- `widgets` removed from config: sends `[]`;
- a misspelt top-level widget key: an error naming it, no request sent;
- import, then one update that settles to the user's JSON.

### 5.4 Acceptance (local stack)

A dashboard with ids, an `insights_vis` widget without `streams`, and an
alarms widget referencing a `honeybadger_alarm` in the same config; add a
widget; reorder; remove one; import (`ImportStateVerifyIgnore: ["widgets"]`,
then a plan that settles it).

## 6. Dependencies

- **honeybadger** (pending batch, bundle `f4003ba0`, uncommitted until Kevin
  okays it): `DashboardWidgetInput.id` kept as sent, unique per dashboard,
  with the character advice; `grid` declares its default; descriptions say
  empty values are dropped; `insights_vis.streams` corrected (left out or
  empty means every stream); `widgets` `default: []` on both inputs;
  deleteDashboard describes the default dashboard. Recorded for honeybadger:
  the provider compares scalar arrays inside widgets as sets, so an
  order-significant scalar array added later needs the spec to say so.
- **api-go** (mcp-v3): `Dashboards` exists (Get, Create, Update, Delete);
  re-vendor after honeybadger commits.

## 7. Out of scope

- Typed widgets, per-widget resources, and validating `config` against its
  per-type schema in the provider (the server validates on apply).
- Listing dashboards (data sources stay deferred, D5).
