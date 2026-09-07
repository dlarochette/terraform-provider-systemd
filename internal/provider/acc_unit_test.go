//go:build !skip_acc

package provider

import (
	"context"
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
	// t.Context() is canceled before Cleanup funcs run, so a safety-net
	// delete here must use context.Background() rather than ctx.
	t.Cleanup(func() {
		if err := c.DeleteUnit(context.Background(), name); err != nil {
			t.Logf("cleanup DeleteUnit %s: %v", name, err)
		}
	})

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

// TestAccVerifyUnit exercises the provider-side systemd-analyze verify flow
// against the real systemd of the guest: a valid unit passes in every mode,
// a broken unit fails in error mode and is rolled back.
func TestAccVerifyUnit(t *testing.T) {
	c := accClient(t)
	ctx := t.Context()

	name := "tf-acc-verify.service"
	good := "[Unit]\nDescription=tf-acc verify\n\n[Service]\nType=oneshot\nExecStart=/bin/true\n"
	broken := "ExecStart=/bin/true no section\n"

	if v, err := c.Host.SystemdVersion(); err == nil {
		t.Logf("guest systemd: %s", v)
	}

	c2 := &Client{Host: c.Host, Verify: VerifyError}
	// broken: must fail and roll back (file absent)
	if _, err := c2.PutUnitVerified(ctx, name, broken, nil, nil); err == nil {
		t.Fatal("expected verification failure for a broken unit")
	}
	if _, err := c.GetUnit(ctx, name); err == nil {
		t.Fatal("broken unit must have been rolled back")
	}

	// valid: writes + warns nothing
	if _, err := c2.PutUnitVerified(ctx, name, good, accBool(false), nil); err != nil {
		t.Fatalf("valid unit must pass verification: %v", err)
	}
	t.Cleanup(func() {
		c.DeleteUnitLifecycle(context.Background(), name)
		_ = c.DeleteUnit(context.Background(), name)
	})
	got, err := c.GetUnit(ctx, name)
	if err != nil {
		t.Fatalf("GetUnit: %v", err)
	}
	if !strings.Contains(got, "ExecStart=/bin/true") {
		t.Errorf("unexpected content: %s", got)
	}

	// warn mode: broken unit surfaces diagnostics but does not fail
	c3 := &Client{Host: c.Host, Verify: VerifyWarn}
	if _, err := c3.PutUnitVerified(ctx, name, broken, nil, nil); err != nil {
		t.Fatalf("warn mode must not fail: %v", err)
	}
	// restore good content for cleanliness
	if _, err := c2.PutUnitVerified(context.Background(), name, good, nil, nil); err != nil {
		t.Logf("restore: %v", err)
	}
}
