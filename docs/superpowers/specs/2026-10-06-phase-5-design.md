# Phase 5: Environments, Sites, Teams and Status Pages: Design

Date: 2026-10-06
Status: Draft for review
Parent: `2026-10-05-terraform-provider-design.md` (the umbrella spec). This
spec covers phase 5: `honeybadger_environment`, `honeybadger_site`,
`honeybadger_team` and `honeybadger_status_page`. It builds on the generator
as phases 2b, 4a and 4b left it.

## 1. Goal

Ship four resources, generated from honeybadger's bundle and api-go's `apiv3`
types with one `resources.yaml` entry each and no hand-written per-resource
code. The generator gains generic Go shapes that api-go's new types use:
UUID scalars and IDs, integer enums, nullable enums and times, and string
maps (section 3). Everything else these schemas use already exists.

The standing rules hold: the generator contains as little Honeybadger domain
logic as possible; a schema that would need a generator exception gets fixed
in the spec first, without making the spec less accurate; and the local
acceptance suite is the gate, with no production smoke test.

## 2. Decisions

| Decision | Choice |
|---|---|
| People | Out of scope. No account or team member resources and no invitations. Members can't be created (they join by accepting an invitation), and invitations send email on apply and drift once accepted. People stay in the web UI. |
| Team project access | `honeybadger_team.project_ids` is the team's whole project list. `honeybadger_project` keeps no team setting (umbrella spec). |
| Site header values | Sensitive. honeybadger marks the `request_headers` values `format: password`, because headers often carry credentials. Plans hide every value, including harmless ones. |
| Docs categories | None for phase 5. Check-ins and uptime are different products, and a one-resource category adds nothing. Only integrations are grouped. |
| Status page domain | Configurable, but the acceptance suite never sets it: domains are unique across Honeybadger and an account can change at most 30 a week. |
| Environment adoption | Notices create environments automatically, so a create often meets an existing name (422). The environment resource's docs show adopting it with an `import` block. |

## 3. Generator extensions

api-go's types for these services use Go shapes the generator hasn't met
(the Fable and Codex spec reviews found them). Each is a generic shape rule
with no domain logic: classification in `gotypes`, conversions in `mapping`
and the provider helpers, rendering in `render`.

### 3.1 UUID scalars and UUID IDs

`Site.Id` is `openapi_types.UUID` (`apiv3.SiteID`), and a status page's
`sites[].site_id` is `format: uuid`. Today the generator handles UUIDs only
as collection elements.

- A `uuid` scalar shape: read as `types.StringValue(x.String())`, written by
  parsing the planned string. Scalar `format: uuid` attributes get the same
  lowercase-UUID validator as UUID elements.
- IDs: `checkService` accepts a UUID-typed path parameter; the wrapper parses
  `state.ID` (and an imported ID) before calling the service, compares the
  returned ID with the zero UUID rather than `""`, and turns a malformed ID
  into an error diagnostic.

### 3.2 Integer enums

`SiteFrequency` is a named `int` with `enum: [1, 2, 5, 15]`.

- An `int_enum` shape, and its nullable form, beside the string `enum`.
- Generic conversions over `~int`, as the string ones are over `~string`.
- `int64validator.OneOf(...)`, the counterpart of the string `OneOf`.

### 3.3 Nullable enums and nullable times

`Nullable[SiteMatchType]` and `Nullable[SiteRequestMethod]` (inputs), and
`Nullable[time.Time]` (environment `created_at`, read) have no shape today.
The `Nullable` case gains `enum` and `time` alongside value, struct and
slice, with matching create, update and read conversions.

### 3.4 String maps

`request_headers` is `additionalProperties: {type: string}` on the wire and
`Nullable[map[string]string]` in Go.

- `facts` reads `additionalProperties` (today it's dropped), including the
  value schema's `format: password`; `setKind` accepts an object with no
  properties when it has string `additionalProperties`, and refuses other
  value types until a schema needs one.
- A `map` kind renders `schema.MapAttribute{ElementType: types.StringType}`,
  Sensitive when the values are `format: password`. A declared `default: {}`
  becomes an empty map default.
- A string-map Go shape: `Nullable[map[string]string]` is mapped; a plain
  `map[string]string` is classified but refused until a schema needs it
  (only build what the bundle uses). Both are distinct from the existing
  `map[string]any` shape that backs nested objects.
- Writes follow the phase 4b collection rules: the whole map on create and on
  any update that changes it; a map set in state and null in the plan is sent
  as null.

### 3.5 Empty object defaults

Status page `features` declares `default: {}` and reads `{}` when nothing is
set. `emptyDefault` today covers collections only; it extends to an object
whose children are all optional: the default is the object with every child
null (`objectdefault.StaticValue`). Any other object default is refused.

### 3.6 Already supported

Write-only top-level fields (status page `password`: never read, Sensitive,
kept from the plan), ordered lists of objects, sets with enum items (site
`locations`), URL patterns, `server_filled`, `replace`, and account-level
resources with no parent ID (as `honeybadger_project`).

## 4. The resources

Attribute names, types, descriptions, defaults and validators come from the
bundle. This section lists only what `resources.yaml` decides and what a
user should know.

### 4.1 `honeybadger_environment`

Project-scoped: `project_id`, `name` (required; changing it replaces the
environment, since the API refuses a rename), `notifications` (default
true), `created_at` (read-only, nullable).

```yaml
environment:
  service: Environments
  replace: [name]
  use_state_for_unknown: [created_at]
```

### 4.2 `honeybadger_site`

Project-scoped: `url` (required, `^https?://`), `name` (server-filled; it
defaults to the URL), `active`, `frequency`, `match_type`, `match`,
`request_method`, `request_body`, `request_headers` (Sensitive map),
`validate_ssl`, `locations` (set; empty means every location),
`outage_threshold`, `timeout`, `links.web` and `created_at` (read-only).

`state` and `last_checked_at` change on their own and are omitted.

The server turns `active` off after 740 consecutive failed checks; the next
apply turns it back on, as the config says. `timeout` needs the account's
uptime-timeout feature, and a `frequency` under the plan's floor is refused;
both surface the API's 403.

```yaml
site:
  service: Sites
  omit:
    state: live data
    last_checked_at: live data
  server_filled: [name]
  use_state_for_unknown: [created_at, links]
```

### 4.3 `honeybadger_team`

Account-level: `name` (required), `project_ids` (set), `account_id`,
`links.web` and `created_at` (read-only). `member_count` changes as people join and is
omitted.

```yaml
team:
  service: Teams
  omit:
    member_count: live data
  use_state_for_unknown: [account_id, created_at, links]
```

### 4.4 `honeybadger_status_page`

Account-level: `name` (required), `domain`, `hide_branding`,
`incidents_enabled`, `message_enabled`, `message_title`, `message_body`,
`search_engine_indexing_disabled`, `google_analytics_id`,
`password_protected`, `username`, `password` (write-only, Sensitive),
`features` (object), `sites` (ordered list of `site_id`, `display_name`,
`description`), `check_ins` (ordered list of `check_in_id`, `display_name`,
`description`), and `account_id`, `url`, `links.web` and `created_at`
(read-only). `domain_verified_at` changes on its own and is omitted.
`incidents_enabled` defaults on create to whether the plan has status page
incidents, so it is server-filled rather than given a static default.

`sites` and `check_ins` reference `honeybadger_site` and
`honeybadger_check_in` IDs, so Terraform orders creation. `features` and
`password_protected` need the advanced status pages plan; without it the
API's 403 surfaces.

```yaml
status_page:
  service: StatusPages
  omit:
    domain_verified_at: live data
  server_filled: [incidents_enabled]
  use_state_for_unknown: [account_id, created_at, url, links]
```

## 5. Testing

### 5.1 Generator

Golden and compiled cases for each new shape: UUID scalar and UUID ID, integer
enum (plain and nullable), nullable string enum, nullable time, and string map
(plain, Sensitive, with `default: {}`); a map with non-string values is
refused.

### 5.2 Unit tests (fake API, real generated resources)

The fake API learns the four resources. Cases:

- site headers: set, change, clear (sent as null); values Sensitive in the plan;
- site locations: order changes plan nothing; leaving it out plans `[]`;
- team `project_ids`: set semantics, clearing;
- status page `password`: sent, never read, kept from state; a change outside
  Terraform isn't seen;
- status page `sites` order: reordering plans an update;
- status page `domain` set and cleared (acceptance never sets it);
- environment import; changing `name` plans a replacement;
- site imported by UUID; a malformed ID is an error, not a panic;
- `frequency = 3` is refused at plan time;
- defaulted fields left out of config: create, then an empty plan; removing a
  configured value plans the default;
- status page `features` left out, set, and cleared;
- status page `sites` with `display_name` null reads back null;
- `match_type = "success"` with `match` set: the API's 422 surfaces (no
  provider validator).

### 5.3 Acceptance (`TF_ACC`, local stack)

Against `http://localhost:3001`:

- environment: create, update, import; adopting an existing environment with
  an `import` block;
- site: create with headers, locations and a match type; update; clear the
  headers; import;
- team: create with `project_ids` pointing at a `honeybadger_project`;
  change the set; import;
- status page: create listing a site and a check-in from the same config;
  reorder; remove one; import. `features` and `password_protected` run when
  the local account has advanced status pages, which Kevin enables;
- integration `site_ids` (the gap phase 4b left): one integration test points
  `site_ids` at a `honeybadger_site`.

## 6. Dependencies

- **honeybadger**: done at `a681addd94` and `897f0b5269` (bundle `cb1c77f2`):
  - declared defaults on SiteInput, StatusPageInput, TeamInput and
    EnvironmentInput, including `[]` for `locations`, `project_ids`, `sites`
    and `check_ins`, and `{}` for `request_headers` and `features`;
  - site reads return non-null `request_method` and `validate_ssl`;
  - `incidents_enabled` has no static default; null on update resets it to
    the plan's value;
  - create accepts `hide_branding` and `password_protected` as false on any
    plan;
  - `features` is one component (`StatusPageFeatures`) with `[string, null]`
    keys; a never-set key is absent from the read, so it reads as null;
  - `uniqueItems` on `project_ids` and `locations`; `format: password` on
    `request_headers` values; named components for the status page's site and
    check-in items;
  - environment `created_at` stays nullable: legacy rows can be null.
- **api-go** (mcp-v3): all four services at `eea3069`, on bundle `cb1c77f2`
  (honeybadger `897f0b5269`, which also names the status page read items
  `StatusPageSite` and `StatusPageCheckIn`).
- **Local account**: advanced status pages and uptime-timeout enabled for the
  feature-gated acceptance steps.

## 7. Out of scope

- Account and team members, account and team invitations.
- Environment bulk update and bulk delete.
- Site checks and outages, status page incidents and incident updates: live
  data, not configuration.
- Data sources (umbrella spec).
