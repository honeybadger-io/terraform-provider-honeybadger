# Contributing

## Layout

The provider lives in `honeybadger/mcp/terraform-provider-honeybadger`, next to
`api-go` and `honeybadger-mcp-server`. `../go.work` lists all three, so a local
build uses your api-go checkout, including commits that aren't pushed. That's
the normal way to work: `go.mod` isn't bumped as api-go moves, so a build
outside the workspace (`GOWORK=off`, or CI) may not compile between releases.
A release bumps `go.mod` to a tagged api-go and builds with `GOWORK=off`.

Design: `docs/superpowers/specs/2026-10-05-terraform-provider-design.md`.

Resource schemas aren't written by hand. `make generate` reads
`openapi/bundled.yaml` and `internal/provider/resources.yaml` and writes each
resource's schema and model to `internal/provider/<name>_resource_gen.go`
(spec section 3.3). To change a schema, change the OpenAPI spec in honeybadger
(then `make update-spec`) or the resource's entry in `resources.yaml`. Never
edit a `_gen.go` file.

## Commands

    make build      # compile
    make test       # unit tests (needs terraform on PATH)
    make lint       # golangci-lint
    make update-spec  # copy honeybadger's bundle into openapi/ (after a spec change)
    make generate   # regenerate docs/ with tfplugindocs; commit the result
    make testacc    # acceptance tests against a Honeybadger stack
    make dev        # build for your own Terraform (below)

## Trying it with your own Terraform

`make dev` builds the provider into `.dev/` and writes `.dev/terraformrc`, a
Terraform CLI config whose `dev_overrides` point `honeybadger-io/honeybadger`
at that build. Point Terraform at it from your Terraform project, for example
with direnv:

    # .envrc in your Terraform project
    export TF_CLI_CONFIG_FILE=~/Projects/honeybadger/mcp/terraform-provider-honeybadger/.dev/terraformrc
    export HONEYBADGER_API_URL=http://localhost:3001
    export HONEYBADGER_API_TOKEN=...   # an API Token for the local stack

Then run `terraform plan` and `apply` as usual; `terraform init` isn't
needed, and Terraform warns that development overrides are in effect. Rerun
`make dev` after pulling changes (from anywhere: `make -C <this repo> dev`).
Leave the directory, or unset `TF_CLI_CONFIG_FILE`, to use providers from the
registry again. `examples/resources/` has a config for every resource.

## Acceptance tests

Acceptance tests create and destroy real objects. Run them against a local
Honeybadger stack, never your everyday account:

    export HONEYBADGER_API_URL=http://localhost:3001
    export HONEYBADGER_API_TOKEN=hba_...   # a token for the terraform-acc account
    make testacc

Alarm tests also need Opticon and Sidekiq running locally: Opticon evaluates
alarms (without it the API answers 503), and Sidekiq sets up a new project's
streams (without it an alarm create waits, then fails).

Before a full run, raise the token account's rate limit in the local stack. A
run makes 150–250 requests; the default is 360 an hour:

    bin/rails runner 'Account.find(ID).update!(api_hits_per_hour: 5000)'

`testAccPreCheck` stops a run early when fewer than 300 requests remain.
Objects are named with a `tf-acc-` prefix; clean up after failed runs with:

    go test ./internal/provider/ -sweep=local

Still to be documented (spec open decision D2): the commands that start the
local stack, and the honeybadger rake task that creates the `terraform-acc`
account and its token.
