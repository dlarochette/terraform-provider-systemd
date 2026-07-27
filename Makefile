.PHONY: build test testacc fmt vet schema

VERSION ?= 0.1.0
LDFLAGS = -X main.Version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/terraform-provider-systemd .

test:
	go test ./...

# Real acceptance tests against a systemd-nspawn machine (root required):
#   sudo apt-get install -y systemd-container debootstrap dbus
#   sudo make testacc
# See scripts/acc-nspawn.sh for env vars (ACC_REBUILD, ACC_WIPE, SYSTEMD_ACC_MACHINE, ...).
testacc:
	./scripts/acc-nspawn.sh

fmt:
	gofmt -s -w .

vet:
	go vet ./...

# Dump provider/resource/data source schemas as JSON (for terraform validate / lint tooling).
schema: build
	bash scripts/dump-schema.sh
