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
| Sources built from | `feat/terraform-3-v3-contract` @ `cd12a844e9` |
| Copied | 2026-10-06 |
| sha256 | `89e7ba2d9bfb1f8e934a8c07280d9f6c708646b8ec75060a086c3100bb5382f2` |
