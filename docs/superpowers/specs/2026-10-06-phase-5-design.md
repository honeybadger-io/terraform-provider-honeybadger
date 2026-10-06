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
code. The generator gains two generic features: string maps and integer
enums. Everything else these schemas use already exists.

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

### 3.1 String maps

An object whose only schema is `additionalProperties: {type: string}` becomes
`schema.MapAttribute{ElementType: types.StringType}`. When the value schema
is `format: password`, the attribute is Sensitive. Writes follow the
collection rules from phase 4b: the whole map is sent on create and on any
update that changes it; a map set in state and null in the plan is sent as
null; a declared `default: {}` becomes an empty map default, so a config that
leaves it out plans the value the read returns.

The Go side is `map[string]string` (or its `Nullable` wrapper). `gotypes`
classifies it as a map shape the same way it classifies slices; a map whose
values aren't strings is refused until a schema needs one.

### 3.2 Integer enums

An integer field with `enum` gets `int64validator.OneOf(...)`, the counterpart
of the string `OneOf` already emitted. Site `frequency` (1, 2, 5, 15) is the
first.

### 3.3 Already supported

Write-only top-level fields (status page `password`: never read, Sensitive,
kept from the plan), ordered lists of objects (status page `sites` and
`check_ins`), sets with enum items (site `locations`), nullable `anyOf`
fields, URL patterns, server-filled fields, and account-level resources with
no parent ID (as `honeybadger_project`).

## 4. The resources

Attribute names, types, descriptions, defaults and validators come from the
bundle. This section lists only what `resources.yaml` decides and what a
user should know.

### 4.1 `honeybadger_environment`

Project-scoped: `project_id`, `name` (required), `notifications` (default
true), `created_at` (read-only).

```yaml
environment:
  service: Environments
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
  omit: [state, last_checked_at]
  server_filled: [name]
  use_state_for_unknown: [created_at, links]
```

### 4.3 `honeybadger_team`

Account-level: `name` (required), `project_ids` (set), `links.web` and
`created_at` (read-only). `member_count` changes as people join and is
omitted.

```yaml
team:
  service: Teams
  omit: [member_count]
  use_state_for_unknown: [created_at, links]
```

### 4.4 `honeybadger_status_page`

Account-level: `name` (required), `domain`, `hide_branding`,
`incidents_enabled`, `message_enabled`, `message_title`, `message_body`,
`search_engine_indexing_disabled`, `google_analytics_id`,
`password_protected`, `username`, `password` (write-only, Sensitive),
`features` (object), `sites` (ordered list of `site_id`, `display_name`,
`description`), `check_ins` (ordered list of `check_in_id`, `display_name`,
`description`), and `url`, `links.web` and `created_at` (read-only).
`domain_verified_at` changes on its own and is omitted.

`sites` and `check_ins` reference `honeybadger_site` and
`honeybadger_check_in` IDs, so Terraform orders creation. `features` and
`password_protected` need the advanced status pages plan; without it the
API's 403 surfaces.

```yaml
status_page:
  service: StatusPages
  omit: [domain_verified_at]
  use_state_for_unknown: [created_at, url, links]
```

## 5. Testing

### 5.1 Generator

Golden and compiled cases for a string map (plain, Sensitive, with `default:
{}`), a map with non-string values (refused), and an integer enum.

### 5.2 Unit tests (fake API, real generated resources)

The fake API learns the four resources. Cases:

- site headers: set, change, clear (sent as null); values Sensitive in the plan;
- site locations: order changes plan nothing; leaving it out plans `[]`;
- team `project_ids`: set semantics, clearing;
- status page `password`: sent, never read, kept from state; a change outside
  Terraform isn't seen;
- status page `sites` order: reordering plans an update;
- status page `domain` set and cleared (acceptance never sets it);
- environment import.

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

- **honeybadger** (v3-terraform-readiness): declared defaults on SiteInput and
  StatusPageInput; nullable `features` children, with the read of an unset key
  stated; `uniqueItems` on `TeamInput.project_ids` and `SiteInput.locations`;
  `format: password` on `request_headers` values; empty defaults on
  `locations`, `project_ids`, status page `sites` and `check_ins`, and on
  `request_headers` if its read returns `{}`; status page nested shapes as
  named components (mcp-v3's request).
- **api-go** (mcp-v3): Environments, Sites and Teams are at `54eed1c`.
  StatusPages ships with the bundle above.
- **Local account**: advanced status pages and uptime-timeout enabled for the
  feature-gated acceptance steps.

## 7. Out of scope

- Account and team members, account and team invitations.
- Environment bulk update and bulk delete.
- Site checks and outages, status page incidents and incident updates: live
  data, not configuration.
- Data sources (umbrella spec).
