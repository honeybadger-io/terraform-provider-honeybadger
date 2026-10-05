default: build

build:
	go build -v ./...

test:
	go test -v -cover -timeout=120s ./...

testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ./...

lint:
	golangci-lint run

generate:
	go generate ./...

.PHONY: default build test testacc lint generate
