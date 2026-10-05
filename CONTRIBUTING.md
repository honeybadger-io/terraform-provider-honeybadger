# Contributing

## Layout

The provider lives in `honeybadger/mcp/terraform-provider-honeybadger`, next to
`api-go` and `honeybadger-mcp-server`. `../go.work` lists all three, so a local
build uses your api-go checkout, including commits that aren't pushed. CI and
releases build with `GOWORK=off` against the api-go version in `go.mod`.

Design: `docs/superpowers/specs/2026-10-05-terraform-provider-design.md`.

## Commands

    make build      # compile
    make test       # unit tests (needs terraform on PATH)
    make lint       # golangci-lint
    make generate   # regenerate docs/ with tfplugindocs; commit the result
    make testacc    # acceptance tests against a Honeybadger stack

Run `GOWORK=off make test` before pushing, to catch code that only builds
against an unpushed api-go.

## Acceptance tests

Acceptance tests create and destroy real objects. Run them against a local
Honeybadger stack, never your everyday account:

    export HONEYBADGER_API_URL=http://localhost:3001
    export HONEYBADGER_API_TOKEN=hba_...   # a token for the terraform-acc account
    make testacc

Alarm tests also need Opticon running locally.

Still to be documented (spec open decision D2): the commands that start the
local stack, and the honeybadger rake task that creates the `terraform-acc`
account and its token.
