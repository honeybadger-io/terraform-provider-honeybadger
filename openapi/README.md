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
| Sources built from | `feat/terraform-5-account-team-tokens` @ `a3bed30db9` |
| Copied | 2026-10-08 |
| sha256 | `559bf9919b0dac4db80d281d963174e67c0ddd5041cde203b2665f9645c85254` |
