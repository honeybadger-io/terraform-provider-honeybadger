#!/bin/sh
# Checks that the provider's copy of the OpenAPI bundle is byte-for-byte the
# one api-go was generated from (spec section 3.3). A mismatch is a warning
# during development; with --release it fails.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
release=false
[ "${1:-}" = "--release" ] && release=true

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

apigo=$(cd "$root" && go list -m -f '{{.Dir}}' github.com/honeybadger-io/api-go)
if [ -z "$apigo" ] || [ ! -f "$apigo/openapi/bundled.yaml.sha256" ]; then
	echo "check-spec: can't find api-go's bundle checksum (is ../go.work in use?)" >&2
	exit 1
fi
want=$(cat "$apigo/openapi/bundled.yaml.sha256")

if [ ! -f "$root/openapi/bundled.yaml" ]; then
	echo "check-spec: openapi/bundled.yaml is missing; run make update-spec" >&2
	exit 1
fi
got=$(sha256 "$root/openapi/bundled.yaml")

if [ "$got" != "$want" ]; then
	echo "check-spec: openapi/bundled.yaml ($got) doesn't match api-go's ($want, $apigo)." >&2
	if $release; then
		echo "A release needs go.mod to require an api-go built from this bundle: bump it to that tagged version." >&2
		exit 1
	fi
	echo "Copy the same honeybadger bundle into both: make update-spec here, and api-go's own update." >&2
	echo "check-spec: warning only; a release fails on this" >&2
	exit 0
fi
echo "check-spec: bundle matches api-go ($got)"
