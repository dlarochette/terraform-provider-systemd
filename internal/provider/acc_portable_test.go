package provider

import (
	"context"
	"strings"
	"testing"
)

func TestAccPortableLifecycle(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()

	// Skip only when portablectl is missing (minimal images); Debian ACC guest has it.
	if _, err := c.Host.ShowPortable("__probe_missing__"); err != nil && strings.Contains(err.Error(), "portablectl") {
		t.Skip("portablectl unavailable")
	}

	name := "tf-acc-portable"
	en, act := true, false
	t.Cleanup(func() {
		_ = c.DeletePortable(context.Background(), name, true)
	})

	if err := c.PutPortable(ctx, name, "local", "/var/tmp/tf-acc-portable.tar", &en, &act); err != nil {
		t.Fatalf("create: %v", err)
	}
	st, err := c.GetPortable(ctx, name)
	if err != nil || !st.ImagePresent || !st.Attached {
		t.Fatalf("read: %+v %v", st, err)
	}
	ufs := strings.ToLower(st.Unit.UnitFileState)
	if !strings.Contains(ufs, "enabled") {
		t.Fatalf("want enabled primary unit, got %+v", st.Unit)
	}

	if err := c.DeletePortable(ctx, name, true); err != nil {
		t.Fatalf("delete: %v", err)
	}
	st, _ = c.GetPortable(ctx, name)
	if st.ImagePresent {
		t.Fatalf("expected image removed: %+v", st)
	}
	if st.Attached {
		t.Fatalf("expected detached after delete: %+v", st)
	}
}
