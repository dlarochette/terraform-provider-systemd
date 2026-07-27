//go:build !skip_acc

package provider

import (
	"strings"
	"testing"
)

// TestAccUnitLifecycle exercises Client.PutUnit / GetUnit / UnitStatus /
// DeleteUnit end to end against a live systemd-nspawn machine: write a
// oneshot unit, enable+start it, assert it loaded and ran, then destroy it
// and assert the unit file is gone.
func TestAccUnitLifecycle(t *testing.T) {
	c := accClient(t)
	ctx := t.Context()

	const name = "tf-acc-demo.service"
	const content = "[Unit]\n" +
		"Description=tf-acc demo unit\n" +
		"\n" +
		"[Service]\n" +
		"Type=oneshot\n" +
		"ExecStart=/bin/true\n" +
		"\n" +
		"[Install]\n" +
		"WantedBy=multi-user.target\n"

	if err := c.PutUnit(ctx, name, content, accBool(true), accBool(true)); err != nil {
		t.Fatalf("PutUnit: %v", err)
	}

	got, err := c.GetUnit(ctx, name)
	if err != nil {
		t.Fatalf("GetUnit: %v", err)
	}
	if !strings.Contains(got, "ExecStart=/bin/true") {
		t.Fatalf("unexpected unit content: %q", got)
	}

	st, err := c.UnitStatus(ctx, name)
	if err != nil {
		t.Fatalf("UnitStatus: %v", err)
	}
	if st.LoadState != "loaded" {
		t.Fatalf("expected LoadState=loaded, got %+v", st)
	}
	// oneshot ExecStart=/bin/true completes immediately; a successful start
	// leaves the unit "active" (briefly) or already "inactive"/"dead" by the
	// time we check. Anything else (e.g. "failed") means start didn't work.
	if st.ActiveState != "active" && st.ActiveState != "inactive" {
		t.Fatalf("expected ActiveState active or inactive after oneshot start, got %+v", st)
	}

	if err := c.DeleteUnit(ctx, name); err != nil {
		t.Fatalf("DeleteUnit: %v", err)
	}

	if _, err := c.GetUnit(ctx, name); err == nil {
		t.Fatal("expected GetUnit to fail after DeleteUnit (unit file should be gone)")
	}
}
