# Terraform Provider for Honeybadger

Manage [Honeybadger](https://www.honeybadger.io) as code: projects, uptime
checks, check-ins, Insights alarms and dashboards, teams, status pages, and 34
alerting integrations, through Honeybadger's Data API (v3).

Write your monitoring next to the service it watches, review it in a pull
request, and stand up a new app's whole Honeybadger setup with one
`terraform apply`.

> **Beta.** The provider isn't on the Terraform Registry yet. Until it is,
> build it locally (see [Trying it now](#trying-it-now)).

## Quick start

```terraform
terraform {
  required_providers {
    honeybadger = {
      source = "honeybadger-io/honeybadger"
    }
  }
}

provider "honeybadger" {
  # Reads HONEYBADGER_API_TOKEN from the environment.
}

resource "honeybadger_project" "app" {
  name     = "My App"
  language = "ruby"
}

# The Ingestion Key your app reports errors with.
resource "honeybadger_ingestion_key" "app" {
  project_id = honeybadger_project.app.id
  label      = "production"
}

# Alert when the nightly job doesn't report in.
resource "honeybadger_check_in" "nightly" {
  project_id    = honeybadger_project.app.id
  name          = "Nightly billing"
  report_period = "1 day"
  grace_period  = "10 minutes"
}

# Check the health endpoint from two continents.
resource "honeybadger_site" "api" {
  project_id = honeybadger_project.app.id
  url        = "https://api.example.com/health"
  locations  = ["Virginia", "Frankfurt"]
  match_type = "include"
  match      = "ok"
}

# Give the backend team access.
resource "honeybadger_team" "backend" {
  name        = "Backend"
  project_ids = [honeybadger_project.app.id]
}

output "ingestion_key" {
  value = honeybadger_ingestion_key.app.key
}

output "check_in_url" {
  value = honeybadger_check_in.nightly.report_url
}
```

```sh
export HONEYBADGER_API_TOKEN=hbu_...
terraform apply
```

[`examples/resources/`](examples/resources) has a working config for every
resource.

## What you can manage

| Area | Resources |
| --- | --- |
| Projects | `honeybadger_project`, `honeybadger_ingestion_key`, `honeybadger_environment` |
| Uptime and cron monitoring | `honeybadger_site`, `honeybadger_check_in` |
| Insights | `honeybadger_alarm`, `honeybadger_dashboard` |
| Access | `honeybadger_team` |
| Status pages | `honeybadger_status_page` |
| Integrations | `honeybadger_integration_<service>`, one resource per service (below) |

Integrations: AlertOps, Asana, Backlog, Bitbucket, Campfire, ClickUp, ClickUp
Chat, Datadog, Discord, email, GitHub, GitLab, Google Chat, iLert,
incident.io, Instatus, Intercom, Jira, Jira Cloud, Linear, Mattermost,
Microsoft Teams, Opsgenie, PagerDuty, PagerTree, Redmine, Rootly, Shortcut,
Slack, Sprintly, Trello, VictorOps, webhooks and Zulip.

Every resource supports `terraform import`, so you can adopt a setup that
already exists.

## Configuration

| Argument | Environment variable | Default |
| --- | --- | --- |
| `api_token` | `HONEYBADGER_API_TOKEN`, then `HONEYBADGER_PERSONAL_AUTH_TOKEN` | none (required) |
| `api_url` | `HONEYBADGER_API_URL` | `https://app.honeybadger.io` |

`api_token` takes a personal API Token (`hbu_`), an account API Token
(`hba_`), or an OAuth access token. Set it through the environment rather than
in config, so it stays out of your repository.

The token needs the scopes of the resources it manages; each resource's docs
list them, and the provider docs have the full table. An account API Token
made before teams could be managed with one needs `team:write` added in its
settings to manage `honeybadger_team`.

EU accounts set `api_url = "https://eu-app.honeybadger.io"`.

## Importing existing objects

Bring something you set up in the Honeybadger UI under Terraform with an
`import` block (Terraform 1.5+):

```terraform
import {
  to = honeybadger_project.app
  id = "Xk9mZp"
}

import {
  to = honeybadger_check_in.nightly
  id = "Xk9mZp/b4Ck9w"
}
```

| ID format | Resources |
| --- | --- |
| `<id>` | `honeybadger_project`, `honeybadger_team`, `honeybadger_status_page` |
| `<project_id>/<id>` | everything else |

Run `terraform plan -generate-config-out=generated.tf` to have Terraform write
the matching config for you.

## Trying it now

Until the provider is on the registry, build it and point Terraform at the
build:

```sh
git clone https://github.com/honeybadger-io/terraform-provider-honeybadger
cd terraform-provider-honeybadger
make dev
export TF_CLI_CONFIG_FILE="$PWD/.dev/terraformrc"
```

Then run `terraform plan` and `terraform apply` in your own project; skip
`terraform init`. Applying changes real objects in your Honeybadger account,
so start in a test project. [CONTRIBUTING.md](CONTRIBUTING.md#trying-it-with-your-own-terraform)
has the details.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/install) 1.0 or later
  (1.5 or later for `import` blocks)
- [Go](https://go.dev/doc/install) 1.27 or later, to build the provider

## How it's built

Resource schemas are generated from Honeybadger's OpenAPI v3 spec, so
attributes, descriptions and validation match the API itself. A new field in
the API becomes a new attribute with `make update-spec && make generate`.
Unit tests run against an in-memory fake of the API, and acceptance tests run
against a real Honeybadger stack before every merge.

## Contributing

Bug reports and pull requests are welcome. See
[CONTRIBUTING.md](CONTRIBUTING.md) for how the code is laid out, how to run the
tests, and how to try a change against your own account.

## License

[Mozilla Public License 2.0](LICENSE)
