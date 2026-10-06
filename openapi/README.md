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
| Sources built from | `feat/terraform-3-v3-contract` @ `b5f086e1cc` |
| Copied | 2026-10-06 |
| sha256 | `2ae645617df9af45cf394008ca5caff399e7d3d55867dfd33643f75ed0a49d4e` |
