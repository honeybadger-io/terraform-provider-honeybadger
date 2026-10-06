# Phase 4b: Integration Resources: Design

Date: 2026-10-06
Status: Draft for review
Parent: `2026-10-05-terraform-provider-design.md` (the umbrella spec). This
spec covers phase 4b: `honeybadger_integration_<type>`. It extends the
generator from phase 4a (`2026-10-05-phase-4a-alarm-design.md`).

## 1. Goal

Ship one resource per creatable integration type, 34 in all, generated from
honeybadger's bundle and api-go's `apiv3` types with one `resources.yaml`
entry and no hand-written per-type code. The generator learns what the
integration schemas need, as generic OpenAPI and Go-shape rules: unions with
a discriminator, nested objects with optional, defaulted and write-only keys,
read-only objects, lists of objects, item enums and UUID elements.

Two rules from Kevin bound the design:

- the generator contains as little Honeybadger domain logic as possible;
- when something needs generator exceptions, question the API or schema
  first, but never make the spec less accurate to suit the generator.

## 2. Decisions

| Decision | Choice |
| --- | --- |
| Shape (umbrella D6) | One resource per type: `honeybadger_integration_<snake(type)>` |
| Names | From the channel class (`type`), snake-cased, version kept: `web_hook`, `ops_genie_v2`, `pager_duty_v2`. Never display titles |
| Type-specific settings | A nested `config` block, typed per type, not flattened |
| Nested object writes | One rule for every nested object (section 3.2); no merge marker |
| `active` | A normal setting on every type, `default: true` (honeybadger be6f1e77dd) |
| Authentication (`connected`) | Omitted: Honeybadger's state, not config |
| OAuth types | Included. Created active and unconnected; they start sending once someone connects them at `links.web` |
| `links.web` | Exposed, as a read-only object `links` |
| Site IDs | Kept as `format: uuid` in the spec; the generator supports UUID elements |
| Legacy types (no create branch) | No resource; reading one through a resource is an error naming it |

## 3. Generator extensions

### 3.1 Unions with a discriminator (`variants`)

A `resources.yaml` entry with `variants: <field>` generates one resource per
branch of the create input's `allOf` whose `if` fixes that field with a
`const`:

- **Name:** `<entry name>_<snake(const)>`. CamelCase splits at each capital,
  and a version stays attached: `OpsGenieV2` → `ops_genie_v2`,
  `IncidentIo` → `incident_io`, `Github` → `github`.
- **Create schema:** the shared create input, narrowed by the branch's
  `then`: its `required` list, its property schemas (`config`'s `$ref`, the
  `events` item enum, `active`'s default), and its bounds. A branch narrows;
  it never widens. A branch property that isn't in the shared input, an item
  enum that isn't a subset of the shared one, or a `const` that isn't one of
  the discriminator's enum values is a generation error.
- **Update and read schemas:** shared by every variant. A variant's `config`
  is typed from its create branch on every side.
- **The discriminator isn't an attribute.** Create sends the branch's const.
  Read checks the returned value and fails with
  `"<id> is a <Type> integration; use honeybadger_integration_<snake(Type)>"`,
  or, for a type with no create branch, `"<id> is a <Type> integration, which
  can't be managed with Terraform"`. The check is in Read, so it covers
  import too, which reads after parsing the ID.

### 3.2 Nested objects

Phase 4a allowed only required keys inside an object. The bundle now has
optional, defaulted and write-only keys inside `config`, so:

- **Optional keys** are plain Optional.
- **Defaulted keys** (`default:` declared) are Optional + Computed with a
  static default, as top-level ones are. Honeybadger declares every fixed
  default, typed as a read returns it (Jira `resolve_transition: "5"`,
  Backlog `enable_webhook: true`). Defaults computed per integration aren't
  declared and don't occur in a creatable type.
- **Write-only keys** (`writeOnly`, secrets) are Sensitive and never read
  back. `FromAPI` takes the prior object and carries its write-only keys
  over, so state keeps the configured secret and removing it is detected.
- **Still a generation error:** a read-only key inside an object, an
  optional key that isn't nullable (it can't be cleared), and a
  server-filled key inside an object.

**One write rule for every nested object.** When the planned object differs
from state, the request carries:

- every key that has a value in the plan, changed or not;
- `null` for a key that's set in state and null in the plan (removed from
  config); only nullable keys can be optional, so this is always valid;
- nothing for a key that's null in both.

This works for both API semantics without a marker. For an object the API
merges (integration `config`: a key left out keeps its value, null clears
it), resending unchanged keys has no effect (honeybadger: per-key hooks fire
only on a change), and a secret never set in Terraform, such as after an
import, is never sent, so it's kept. For an object the API replaces whole
(alarm `trigger_config`, all keys required), it's the full object, as in 4a.

A known object holding an unknown value is still an error diagnostic, never
a zero value (4a, section 3.1).

**Optional objects.** An object that's optional and not nullable on update
(Slack's `config`, by design: it has no required settings) follows the
existing scalar rule: Optional + Computed with `UseStateForUnknown`, so
leaving it out keeps what's stored.

### 3.3 Read-only objects

A read-only object property (`links`) becomes a Computed
`SingleNestedAttribute` with Computed children, read with the same
`FromAPI` converter and never written. Resources that don't want one omit it,
as before.

### 3.4 Collections

- **Lists of objects** (`filters`): a `ListNestedAttribute` whose element
  object follows section 3.2. Written whole when it changes.
- **Item enums** (`events`): element validators from the items' `enum`.
  Branch item enums narrow the shared one.
- **`maxItems: 0`** (Intercom's `events`): a `SizeAtMost(0)` validator, so
  setting events fails at plan.
- **UUID elements** (`format: uuid`): string elements in Terraform, with a
  UUID validator; converted to and from api-go's `openapi_types.UUID`.

### 3.5 Go shapes

Added to the shape table, all matched by Go type as before:

| Go shape | Write | Read |
| --- | --- | --- |
| `*bool` | create: pointer when known; update: pointer when changed (attributes that can't be null only) | `boolPtrValue` |
| `*map[string]any`, `map[string]any` for an object attribute | `toMap`: the plan's object, by the rule in 3.2, keyed by attribute name | `fromMap` into the typed object; keys the schema doesn't know are ignored |
| slices of an enum, of a struct, of `openapi_types.UUID` | as 4a's slices, element by element | as 4a's slices |

`toMap` and `fromMap` are generic: JSON-compatible values keyed by the
spec's property names, so the untyped map api-go uses needs no per-type Go
code.

### 3.6 Rename guard

Generation fails when a committed `*_resource_gen.go` would be deleted,
unless `resources.yaml` lists the resource under `retired:` (a list kept for
the record). A renamed or dropped integration type then breaks loudly.
`make generate` checks it against the files on disk, which are committed.

## 4. The resources

```yaml
integration:
  create: createIntegration
  read: getIntegration
  update: updateIntegration
  delete: deleteIntegration
  import: "<project_id>/<id>"
  variants: type
  service: Integrations
  api: {model: Integration, create_params: IntegrationCreateParams, update_params: IntegrationUpdateParams}
  find_hint: "the project's integrations in Honeybadger"
  omit:
    error: live data
    error_reason: live data
    connected: authentication status, Honeybadger's state rather than config
  server_filled: [events]
```

Each resource's schema:

| Attribute | Kind |
| --- | --- |
| `project_id` | Required, forces replacement |
| `active` | Optional + Computed, default true |
| `events` | Optional + Computed set, the type's item enum; server-filled, since the default is the type's events filtered by the account's plan |
| `filters` | Optional list of objects (`event`, `query`) |
| `site_ids` | Optional set of UUIDs |
| `check_in_ids`, `alarm_alert_ids`, `alarm_ok_ids` | Optional sets |
| `all_sites`, `all_check_ins` | Optional + Computed, default false |
| `included_environments`, `excluded_environments` | Optional sets |
| `rate`, `threshold`, `notification_limit` | Optional |
| `config` | The type's settings (section 3.2); required for 33 types, optional for Slack |
| `links` | Computed object with `web`, the integration's page, where an OAuth type is connected |
| `id`, `created_at` | Computed |

Notes for the docs:

- **OAuth types** (GitHub, GitLab, Bitbucket, Asana, Jira Cloud, Trello,
  Linear, Slack, Intercom, ClickUp, ClickUp Chat) are created unconnected.
  They send nothing until someone connects them at `links.web`; connecting
  never changes `active`, so it causes no diff. GitHub attaches its App
  installation when someone with access saves it or the App is installed on
  the repo.
- **Managed lists drift** when sites or check-ins are added, unless
  `all_sites` / `all_check_ins` is set (umbrella section 7).
- **Webhook URLs** are re-resolved on every save, so an update fails if the
  URL's host doesn't resolve.

## 5. Testing

### 5.1 Generator (golden, compiled)

The fixture gains a three-branch union:

- branch 1: a write-only secret in `config`, a list of objects and an item
  enum;
- branch 2: defaulted keys in `config`, and an optional, non-nullable
  `config`;
- branch 3: `events` with `maxItems: 0`;
- shared: a UUID set, a `*bool`, and a read-only `links` object.

Error cases, each naming the resource and field: an optional non-nullable
key inside an object, a server-filled key inside an object, a branch whose
`if` isn't a `const` on the variant field, a branch that widens the shared
schema, and a committed resource disappearing without a `retired` entry.

### 5.2 Unit tests (fake API, real generated resources)

- `web_hook`: the create body; changing one `config` key resends every set
  key; removing `bearer_token` sends null; a secret never set in Terraform
  is never sent; a read without the secret plans no diff; `events` is filled
  by the server; `filters` round-trips; a malformed `site_ids` UUID fails at
  plan.
- A Slack integration read through `web_hook` fails naming
  `honeybadger_integration_slack`; a legacy type fails naming it.
- Intercom with `events` set fails at plan.
- Slack with `config` left out plans no diff; created with `active = true`
  while unconnected; `links.web` is set.
- A type with a defaulted key: removing it plans and sends the default.

### 5.3 Acceptance (`TF_ACC`, local stack)

- `TestAccIntegrationWebHook`: the full lifecycle, every common setting
  except `site_ids` (no site resource until phase 5), `config` changes,
  removing the secret, import, and deleted outside Terraform. WebHook
  resolves its URL's host on every save, so the test uses a public IP
  literal, which skips the lookup: `https://93.184.216.34/hook`.
- `TestAccIntegrationEmail`: create, update, import.
- `TestAccIntegrationSlack`: an OAuth type created active and unconnected;
  the plan is empty afterwards and `links.web` is set.
- `TestAccIntegrationTypes`: a create-read-update-delete check over every
  type that creates without an external call, so a type whose read doesn't
  match its schema fails here rather than in a user's apply. From
  honeybadger's reading of the channel code, that's 33 of the 34; only
  IncidentIo calls out (incident.io) on every create. Each type uses the
  minimal config honeybadger listed, kept in the test's data:
  - WebHook uses an IP-literal URL; Zulip uses `zulip_domain`, not
    `server_url` (which triggers DNS); Backlog sets `enable_webhook: false`
    (otherwise a save registers a Backlog webhook);
  - GitHub runs with `active: false`, so it doesn't try to attach the App,
    and leaves out `issue_fields`;
  - GitHub, GitLab and Bitbucket allow one per project and Intercom two, so
    each type gets its own resource in one project.

  The update step changes one optional `config` key or `active`, to cover
  the write rule on every type.

## 6. Dependencies

- **Bundle:** honeybadger `93ab2dd1` (080546461e) or later: `active`
  defaults on every branch, declared `config` defaults, the read wording,
  Intercom's `maxItems: 0`.
- **api-go:** regenerated from the same bundle (mcp-v3), so `make
  check-spec` matches.
- **honeybadger, answered:** which types create without an external call,
  and their minimal configs (section 5.3).

## 7. Out of scope

- Sites, so `site_ids` acceptance (phase 5).
- Data sources (umbrella D5).
- Integration actions other than configuration: test notifications,
  connecting OAuth.
- Legacy types without a create branch.
