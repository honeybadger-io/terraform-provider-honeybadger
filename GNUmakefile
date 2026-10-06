default: build

build:
	go build -v ./...

test:
	go test -v -cover -timeout=120s ./...

testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ./...

lint:
	golangci-lint run

update-spec:
	./scripts/update-spec.sh

check-spec:
	./scripts/check-spec.sh

# Phase 7's release checklist runs this: a release needs the provider and the
# api-go its go.mod requires (not the local checkout go.work points at) built
# from the same bundle.
check-spec-release:
	GOWORK=off ./scripts/check-spec.sh --release

# specgen before tfplugindocs: the docs are built from the generated schemas.
generate: check-spec
	go generate ./internal/...
	go generate .

.PHONY: default build test testacc lint generate update-spec check-spec check-spec-release
