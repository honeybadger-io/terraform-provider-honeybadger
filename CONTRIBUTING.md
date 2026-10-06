# Contributing

## Layout

The provider lives in `honeybadger/mcp/terraform-provider-honeybadger`, next to
`api-go` and `honeybadger-mcp-server`. `../go.work` lists all three, so a local
build uses your api-go checkout, including commits that aren't pushed. That's
the normal way to work: `go.mod` isn't bumped as api-go moves, so a build
outside the workspace (`GOWORK=off`, or CI) may not compile between releases.
A release bumps `go.mod` to a tagged api-go and builds with `GOWORK=off`.

Design: `docs/superpowers/specs/2026-10-05-terraform-provider-design.md`.

## Commands

    make build      # compile
    make test       # unit tests (needs terraform on PATH)
    make lint       # golangci-lint
    make update-spec  # copy honeybadger's bundle into openapi/ (after a spec change)
    make generate   # regenerate docs/ with tfplugindocs; commit the result
    make testacc    # acceptance tests against a Honeybadger stack

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
