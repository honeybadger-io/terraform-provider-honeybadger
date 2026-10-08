# OpenAPI bundle

`bundled.yaml` is honeybadger's generated v3 bundle (`bin/rails openapi:bundle`),
copied by `make update-spec`. Don't edit it: fix the spec in honeybadger, then
copy the new bundle into every downstream project (api-go, docs, this provider).

The sha256 identifies the bundle. In honeybadger the bundle is a generated,
gitignored file, so the branch and commit identify only the source
definitions it was built from.

| | |
| --- | --- |
| Source | `honeybadger/openapi/v3/bundled.yaml` |
| Sources built from | `feat/terraform-5-account-team-tokens` @ `980765b644` |
| Copied | 2026-10-08 |
| sha256 | `fd8543ff00c0e483a6379d0b232ac216c812cf5a6096771f415412b635d50ec1` |
