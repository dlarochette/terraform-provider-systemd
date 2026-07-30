.PHONY: build test testacc testacc-ssh fmt vet schema

VERSION ?= 0.1.0
LDFLAGS = -X main.Version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/terraform-provider-systemd .

test:
	go test ./...

# Real acceptance tests against a systemd-nspawn machine.
# Elevates only the harness (nspawn/debootstrap); go test runs as SUDO_USER
# so GOCACHE/GOMODCACHE stay owned by you, not root.
#   make testacc
# See scripts/acc-nspawn.sh for env vars (ACC_REBUILD, ACC_WIPE, SYSTEMD_ACC_MACHINE, ...).
testacc:
	@if [ "$$(id -u)" -eq 0 ]; then \
		./scripts/acc-nspawn.sh; \
	else \
		sudo ./scripts/acc-nspawn.sh; \
	fi

# Same guest, but TestAcc* over SSH/SFTP (remote.Dial) — see scripts/acc-nspawn-ssh.sh.
testacc-ssh:
	@if [ "$$(id -u)" -eq 0 ]; then \
		./scripts/acc-nspawn-ssh.sh; \
	else \
		sudo ./scripts/acc-nspawn-ssh.sh; \
	fi

fmt:
	gofmt -s -w .

vet:
	go vet ./...

# Dump provider/resource/data source schemas as JSON (for terraform validate / lint tooling).
schema: build
	bash scripts/dump-schema.sh
