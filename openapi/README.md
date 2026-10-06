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
| Sources built from | `feat/terraform-3-v3-contract` @ `8126a6fd3b` |
| Copied | 2026-10-05 |
| sha256 | `cfe849548c703f3dc2a19006b9d0e05da1f47c0c35b314884c8909c6c0f6c410` |
