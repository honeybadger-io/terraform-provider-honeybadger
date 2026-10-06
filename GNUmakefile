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

# A local build for your own Terraform: the binary and a CLI config whose
# dev_overrides point at it, both in .dev/. Point Terraform at the config with
# TF_CLI_CONFIG_FILE (CONTRIBUTING.md); rerun after pulling changes.
DEV_DIR := $(CURDIR)/.dev

dev:
	mkdir -p $(DEV_DIR)
	go build -o $(DEV_DIR)/terraform-provider-honeybadger .
	printf 'provider_installation {\n  dev_overrides {\n    "honeybadger-io/honeybadger" = "%s"\n  }\n  direct {}\n}\n' "$(DEV_DIR)" > $(DEV_DIR)/terraformrc
	@echo
	@echo "Built $(DEV_DIR)/terraform-provider-honeybadger. To use it, add this to your Terraform project's .envrc:"
	@echo "  export TF_CLI_CONFIG_FILE=$(DEV_DIR)/terraformrc"

# Phase 7's release checklist runs this: a release needs the provider and the
# api-go its go.mod requires (not the local checkout go.work points at) built
# from the same bundle.
check-spec-release:
	GOWORK=off ./scripts/check-spec.sh --release

# specgen before tfplugindocs: the docs are built from the generated schemas.
generate: check-spec
	go generate ./internal/...
	go generate .

.PHONY: default build dev test testacc lint generate update-spec check-spec check-spec-release
