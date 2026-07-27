//go:build !skip_acc

package provider

import (
	"strings"
	"testing"
)

// TestAccNetworkReload exercises Client.PutNetwork / GetNetwork / DeleteNetwork
// against the live nspawn machine, matched to the loopback interface so
// NetworkReload has a real link to apply to.
func TestAccNetworkReload(t *testing.T) {
	c := accClient(t)
	ctx := t.Context()

	// The minimal debootstrap image doesn't run systemd-networkd by default
	// (`networkctl reload` fails with "Unit dbus-org.freedesktop.network1.service
	// not found" otherwise). Bring it up via the same Client surface the
	// provider itself uses, and leave it stopped again afterwards.
	if err := c.ApplyUnitLifecycle(ctx, "systemd-networkd.service", accBool(true), accBool(true)); err != nil {
		t.Fatalf("start systemd-networkd: %v", err)
	}
	t.Cleanup(func() { c.DeleteUnitLifecycle(ctx, "systemd-networkd.service") })

	const filename = "90-tf-acc.network"
	const content = "[Match]\n" +
		"Name=lo\n" +
		"\n" +
		"[Network]\n" +
		"Description=tf-acc test network\n"

	// PutNetwork writes the file and runs `networkctl reload`; a non-nil
	// error here means NetworkReload itself failed.
	if err := c.PutNetwork(ctx, filename, content); err != nil {
		t.Fatalf("PutNetwork: %v", err)
	}
	t.Cleanup(func() {
		if err := c.DeleteNetwork(ctx, filename); err != nil {
			t.Logf("cleanup DeleteNetwork %s: %v", filename, err)
		}
	})

	got, err := c.GetNetwork(ctx, filename)
	if err != nil {
		t.Fatalf("GetNetwork: %v", err)
	}
	if !strings.Contains(got, "Name=lo") {
		t.Fatalf("unexpected network content: %q", got)
	}

	if _, err := c.LinkStatus(ctx, "lo"); err != nil {
		t.Fatalf("LinkStatus: %v", err)
	}
}
