package provider

import (
	"context"
	"strings"
	"testing"
)

func TestAccMachineLifecycle(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	name := "tf-acc-nested"
	// Nested nspawn cannot mount cgroups inside the ACC guest even with
	// Capability=all on the outer machine — so ACC exercises image + .nspawn +
	// enable, not an active nested container start.
	settings := "[Exec]\nBoot=no\nPrivateUsers=no\nParameters=/usr/bin/sleep infinity\n\n[Network]\nVirtualEthernet=no\n"
	en, act := true, false
	t.Cleanup(func() {
		_ = c.DeleteMachine(context.Background(), name, true)
	})

	if err := c.PutMachine(ctx, name, "local", "/var/tmp/tf-acc-mini.tar", &settings, &en, &act); err != nil {
		t.Fatalf("create: %v", err)
	}
	st, err := c.GetMachine(ctx, name)
	if err != nil || !st.ImagePresent {
		t.Fatalf("read: %+v %v", st, err)
	}
	if !strings.Contains(st.Settings, "Boot=no") {
		t.Fatalf("settings missing Boot=no: %q", st.Settings)
	}
	ufs := strings.ToLower(st.Unit.UnitFileState)
	if !strings.Contains(ufs, "enabled") {
		t.Fatalf("want enabled unit file state, got %+v", st.Unit)
	}

	settings2 := settings + "\n[Exec]\nNotifyReady=no\n"
	if err := c.PutMachine(ctx, name, "local", "/var/tmp/tf-acc-mini.tar", &settings2, &en, &act); err != nil {
		t.Fatalf("update: %v", err)
	}
	st, err = c.GetMachine(ctx, name)
	if err != nil || !strings.Contains(st.Settings, "NotifyReady=no") {
		t.Fatalf("updated settings: %+v %v", st, err)
	}

	if err := c.DeleteMachine(ctx, name, true); err != nil {
		t.Fatalf("delete: %v", err)
	}
	st, _ = c.GetMachine(ctx, name)
	if st.ImagePresent {
		t.Fatal("image should be removed after delete_image=true")
	}
}
