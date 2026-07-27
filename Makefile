.PHONY: build test testacc fmt vet schema

VERSION ?= 0.1.0
LDFLAGS = -X main.Version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/terraform-provider-systemd .

test:
	go test ./...

testacc:
	TF_ACC=1 go test ./internal/provider/ -v -timeout 30m

fmt:
	gofmt -s -w .

vet:
	go vet ./...

# Dump provider/resource/data source schemas as JSON (for terraform validate / lint tooling).
schema: build
	bash scripts/dump-schema.sh
