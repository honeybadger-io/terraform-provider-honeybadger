# Phase 7a: Registry Docs: Design

Date: 2026-10-06
Status: Draft for review (Fable and Codex reviews folded in)
Parent: `2026-10-05-terraform-provider-design.md` (the umbrella spec). This
spec covers the docs part of phase 7, done ahead of signing and publishing.

## 1. Goal

Make every registry page complete and accurate for a Terraform user, from
the same sources as today: honeybadger's bundle and `resources.yaml`.

Measured on the 45 resource pages before this phase: no page has an Import
section; valid values and defaults appear only by chance in prose; no page
names its API scopes; 13 have an example (31 integration types have none);
34 attributes have no description; resource summaries are thin; about 300
descriptions carry request-building instructions ("Send null or [] to
clear"); and a sentence the generator appends is joined without a full stop.

## 2. Decisions

| Decision | Choice |
|---|---|
| Description source | The spec, for API callers and Terraform users alike (Kevin): Terraform is an abstraction over the API, so accurate API facts ("403 `feature_unavailable`", "is 422") stay; Terraform users hit the same errors. No provider-side description overrides and no pattern stripping. |
| Request-building wording | Fixed at the source: honeybadger rephrases "Send null…" instructions as behaviour ("Defaults to 5", "Empty clears it"), which reads correctly for both audiences. `stripCallerWording` is then removed from specgen. |
| Spec gaps | Fixed at the source wherever possible (Kevin): missing descriptions, thin summaries, weak shared descriptions, realistic `example:` values, branch-required keys expressed where the generator can read them. |
| Examples | Generated from the spec's `example:` values where no hand-written example exists; hand-written examples are never overwritten. |

## 3. Generator additions

All generic: they read the bundle, `resources.yaml` and the example files,
never a resource's name. Every sentence the generator writes is joined to
the text before it with terminal punctuation (3.6).

### 3.1 Import

For every generated resource, specgen writes two files in
`examples/resources/<type>/` (`<type>` = `<provider>_<name>`):

- `import-by-string-id.tf`: `import { to = <type>.<label>, id = "<format>" }`;
- `import.sh`: `terraform import <type>.<label> "<format>"`, the ID quoted
  so its placeholders (`<project_id>/<id>`) aren't shell redirection.

`<format>` is the `import` format from `resources.yaml`. `<label>` is the
label of the first `resource "<type>"` block in that directory's
`resource.tf` (hand-written or generated), so the import names the same
resource the example declares (`honeybadger_alarm.errors`), or `example` when
there's no `resource.tf`.

When a resource has write-only attributes (never read back), a generated
sentence follows: "Import can't read `password` back: set it in config."
(the umbrella spec's promise that import docs name what it can't recover).

Both the generated category template (`docsTemplate` in `generate.go`, used
by the 31 integration pages) and the six hand-written templates (alarm,
dashboard, environment, site, status_page, team) gain tfplugindocs v0.25's
import block (`HasImportIDConfig` → `tffile .ImportIDConfigFile`,
`HasImport` → `codefile "shell" .ImportFile`). `docsTemplate` also moves to
the default template's `.HasExamples`/`.ExampleFiles`.

### 3.2 Valid values and defaults

Appended to each attribute's description, in Merge, after the spec's text and
before the server-filled sentence, in one place:

- "Valid values: `a`, `b`." from `enum` (or, for a list or set, its items'
  `enum`);
- "Defaults to `5`." from a declared default; an empty list, set or map
  reads "Defaults to empty."; a `json_subset` default is shown as its JSON.

### 3.3 Required scopes

Appended to the resource description: "Required scopes: `a:read`,
`a:write`." from the scopes of the resource's create, read, update and
delete operations, sorted and de-duplicated. An operation without its own
`security` uses the document's; a resource whose operations name no scopes
gets no line. (The description is tfplugindocs' only channel, so the line
also appears in the page's front-matter description.)

### 3.4 Provider page

The provider page (`templates/index.md.tmpl`, hand-written) gains a generated
table of every resource and its required scopes, written by specgen to a
partial the template includes, and a sentence on API Tokens: an account API
Token needs the scopes of the resources it manages.

### 3.5 Examples

**Values.** `Field` gains `Example any`, read from the schema's `example`
(an array's own `example` wins over its items'), carried through branch
narrowing (a variant's `then` keeps its examples) and Merge into
`Attribute.Example`. An example that is a placeholder, a string matching
`^<.*>$` such as `"<Label>"`, counts as missing (a generic rule).

**Files.** Where `examples/resources/<type>/resource.tf` doesn't exist, or
exists and starts with the generated header, specgen writes one:

- Header comment: `# A minimal configuration, generated from the API spec's
  examples.` It's visible on the page, so it's written for readers; it also
  marks the file as generated.
- One `resource "<type>" "example"` block with every required attribute, and
  the required keys of nested objects (an integration's `config`), using
  their example values.
- A parent path parameter `<x>_id` becomes a reference to the parent's
  example: `<provider>_<x>.<label>.id`, where `<x>` is a `resources.yaml`
  entry and `<label>` is the first block label in the parent's
  `resource.tf` (`honeybadger_project.app`); otherwise the parameter's
  `example:`.
- A sensitive value becomes `var.<name>_<attribute>` (`<name>` the resource
  name without the provider prefix), so names never collide across
  examples, with a matching `variable` block marked `sensitive`.
- A required attribute without a usable example is a generation warning
  naming it; the file is still written without it.

Known limit: kin-openapi drops `required:` written beside a `$ref`, so a
variant branch's `config.required` (Slack's) isn't seen; those keys are
missing from generated examples until honeybadger moves them into the
config schemas (section 5).

### 3.6 Sentence joins

All appended sentences (server-filled, values, defaults, scopes, import
caveat) go through one join that adds terminal punctuation, so "Display name
for the site Removing it…" reads "Display name for the site. Removing it…".

### 3.7 Removed

`stripCallerWording` and its tests, once honeybadger's rephrasing lands.

## 4. Testing

- Generator unit tests:
  - import files for a project-scoped and an account-scoped format, with
    the label taken from an existing `resource.tf` and the `example`
    fallback;
  - the write-only import sentence;
  - each appended sentence and the join, including a `json_subset` default;
  - scopes across operations, the document-level fallback, and no line when
    none;
  - examples: a parent reference using the parent's label, a namespaced
    sensitive variable, a placeholder example treated as missing (warning),
    a variant branch's example values, a hand-written file left alone, a
    generated one regenerated.
- After `make generate`, a test reads `docs/resources/*.md` and checks, for
  representative resources (project, site, status_page,
  integration_web_hook), the exact import block and command, and the exact
  sorted scope line; and that every page has an Import section.
- A test builds the provider into a temp directory with its own
  `dev_overrides` CLI config and runs `terraform validate` on all examples
  together; it declares any `var.x` the examples use but don't declare, and
  skips when `terraform` isn't on PATH (CI installs it).

## 5. Dependencies

- **honeybadger** (sent): rephrase request-building sentences as behaviour;
  describe the 34 undescribed properties; richer model descriptions for
  CheckIn, Dashboard, Environment, Project, Site and Team; better
  `project_id` and integration `created_at` descriptions; realistic
  `example:` values (not `"<Label>"` placeholders) on every required
  create-input property, including integration branches' config keys.
- **honeybadger** (to send): move each integration branch's `config.required`
  from beside the `$ref` into the config schema itself (or an `allOf`), so
  required config keys are readable; give `rate` an `enum` if its values are
  fixed.
- Until those land, generated examples carry warnings and
  `stripCallerWording` stays.

## 6. Out of scope

- Signing, registry publishing, the tagged api-go bump and
  `check-spec-release` (the rest of phase 7).
- Guides beyond the provider page.
