//go:build !skip_acc

package provider

import (
	"strings"
	"testing"
)

// TestAccDropinLifecycle exercises Client.PutDropin / GetDropin / DeleteDropin
// against a throwaway unit on the live nspawn machine.
func TestAccDropinLifecycle(t *testing.T) {
	c := accClient(t)
	ctx := t.Context()

	const unit = "tf-acc-dropin-demo.service"
	const unitContent = "[Unit]\n" +
		"Description=tf-acc dropin target\n" +
		"\n" +
		"[Service]\n" +
		"Type=oneshot\n" +
		"ExecStart=/bin/true\n"

	if err := c.PutUnit(ctx, unit, unitContent, nil, nil); err != nil {
		t.Fatalf("PutUnit: %v", err)
	}
	t.Cleanup(func() {
		if err := c.DeleteUnit(ctx, unit); err != nil {
			t.Logf("cleanup DeleteUnit %s: %v", unit, err)
		}
	})

	const dropin = "override.conf"
	const dropinContent = "[Service]\nEnvironment=TF_ACC=1\n"

	if err := c.PutDropin(ctx, unit, dropin, dropinContent); err != nil {
		t.Fatalf("PutDropin: %v", err)
	}

	got, err := c.GetDropin(ctx, unit, dropin)
	if err != nil {
		t.Fatalf("GetDropin: %v", err)
	}
	if !strings.Contains(got, "TF_ACC=1") {
		t.Fatalf("unexpected dropin content: %q", got)
	}

	if err := c.DeleteDropin(ctx, unit, dropin); err != nil {
		t.Fatalf("DeleteDropin: %v", err)
	}
	if _, err := c.GetDropin(ctx, unit, dropin); err == nil {
		t.Fatal("expected GetDropin to fail after DeleteDropin")
	}
}
