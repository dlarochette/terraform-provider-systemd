//go:build !skip_acc

package provider

import (
	"os"
	"testing"

	"github.com/dlarochette/terraform-provider-systemd/internal/remote"
)

// accSkip skips the calling test unless TF_ACC is set, mirroring
// terraform-plugin-testing's convention so `go test ./...` stays fast and
// green without a live systemd-nspawn machine.
func accSkip(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
}

// accClient returns a Client bound to the systemd-nspawn machine named by
// SYSTEMD_ACC_MACHINE (default "tf-systemd-acc"), started via
// scripts/acc-nspawn.sh. Skips the calling test when TF_ACC is unset.
func accClient(t *testing.T) *Client {
	t.Helper()
	accSkip(t)
	machine := os.Getenv("SYSTEMD_ACC_MACHINE")
	if machine == "" {
		machine = "tf-systemd-acc"
	}
	return &Client{Host: remote.NewNspawn(machine)}
}

// accBool returns a pointer to b, for the *bool enable/active params of
// Client.PutUnit / ApplyUnitLifecycle.
func accBool(b bool) *bool {
	return &b
}
