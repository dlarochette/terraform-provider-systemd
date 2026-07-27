//go:build !skip_acc

package provider

import "testing"

// TestAccInstanceLifecycle writes a template unit (`tf-acc@.service`) and
// exercises Client.ApplyUnitLifecycle / DeleteUnitLifecycle on one of its
// instances (`tf-acc@x.service`), mirroring what systemd_instance does.
func TestAccInstanceLifecycle(t *testing.T) {
	c := accClient(t)
	ctx := t.Context()

	const template = "tf-acc@.service"
	const content = "[Unit]\n" +
		"Description=tf-acc instance %i\n" +
		"\n" +
		"[Service]\n" +
		"Type=oneshot\n" +
		"ExecStart=/bin/true\n" +
		"\n" +
		"[Install]\n" +
		"WantedBy=multi-user.target\n"

	if err := c.PutUnit(ctx, template, content, nil, nil); err != nil {
		t.Fatalf("PutUnit template: %v", err)
	}
	t.Cleanup(func() {
		if err := c.DeleteUnit(ctx, template); err != nil {
			t.Logf("cleanup DeleteUnit %s: %v", template, err)
		}
	})

	instance, err := InstantiateUnit(template, "x")
	if err != nil {
		t.Fatalf("InstantiateUnit: %v", err)
	}
	if instance != "tf-acc@x.service" {
		t.Fatalf("expected tf-acc@x.service, got %q", instance)
	}

	if err := c.ApplyUnitLifecycle(ctx, instance, accBool(true), accBool(true)); err != nil {
		t.Fatalf("ApplyUnitLifecycle: %v", err)
	}
	t.Cleanup(func() { c.DeleteUnitLifecycle(ctx, instance) })

	st, err := c.UnitStatus(ctx, instance)
	if err != nil {
		t.Fatalf("UnitStatus: %v", err)
	}
	if st.LoadState != "loaded" {
		t.Fatalf("expected LoadState=loaded for %s, got %+v", instance, st)
	}
	if st.ActiveState != "active" && st.ActiveState != "inactive" {
		t.Fatalf("expected ActiveState active or inactive for %s, got %+v", instance, st)
	}
}
