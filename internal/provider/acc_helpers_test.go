//go:build !skip_acc

package provider

import (
	"os"
	"strconv"
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

// accClient returns a Client bound to either:
//   - remote SSH when SYSTEMD_ACC_SSH_HOST is set (production Dial path), or
//   - the systemd-nspawn machine named by SYSTEMD_ACC_MACHINE (default
//     "tf-systemd-acc"), started via scripts/acc-nspawn.sh.
//
// Skips the calling test when TF_ACC is unset.
func accClient(t *testing.T) *Client {
	t.Helper()
	accSkip(t)

	if host := os.Getenv("SYSTEMD_ACC_SSH_HOST"); host != "" {
		port := 22
		if p := os.Getenv("SYSTEMD_ACC_SSH_PORT"); p != "" {
			n, err := strconv.Atoi(p)
			if err != nil || n < 1 || n > 65535 {
				t.Fatalf("invalid SYSTEMD_ACC_SSH_PORT %q", p)
			}
			port = n
		}
		user := os.Getenv("SYSTEMD_ACC_SSH_USER")
		if user == "" {
			user = "root"
		}
		keyPath := os.Getenv("SYSTEMD_ACC_SSH_KEY")
		if keyPath == "" {
			t.Fatal("SYSTEMD_ACC_SSH_KEY is required when SYSTEMD_ACC_SSH_HOST is set")
		}
		insecure := os.Getenv("SYSTEMD_ACC_SSH_INSECURE") == "1" ||
			os.Getenv("SYSTEMD_ACC_SSH_INSECURE") == "true"
		cfg := remote.Config{
			Host:                  host,
			User:                  user,
			Port:                  port,
			PrivateKeyPath:        keyPath,
			UseSSHAgent:           false,
			InsecureIgnoreHostKey: insecure,
		}
		sshHost, err := remote.Dial(cfg)
		if err != nil {
			t.Fatalf("SSH Dial %s@%s:%d: %v", user, host, port, err)
		}
		t.Cleanup(func() { _ = sshHost.Close() })
		return &Client{Host: sshHost}
	}

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
