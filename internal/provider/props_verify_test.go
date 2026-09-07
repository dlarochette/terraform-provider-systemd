package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/dlarochette/terraform-provider-systemd/internal/remote"
)

const goodUnit = "[Unit]\nDescription=x\n\n[Service]\nType=oneshot\n\n[Install]\nWantedBy=multi-user.target\n"

// malformed INI: the fake host only detects structural errors, like
// systemd-analyze would detect semantic ones.
const brokenUnit = "ExecStart=/bin/true\nkey outside section\n"

func newVerifyClient(mode string) (*Client, *remote.Fake) {
	f := remote.NewFake()
	return &Client{Host: f, Verify: mode}, f
}

func TestPutUnitVerifiedOff(t *testing.T) {
	c, f := newVerifyClient(VerifyOff)
	n := len(f.Commands)
	if _, err := c.PutUnitVerified(context.Background(), "a.service", brokenUnit, nil, nil); err != nil {
		t.Fatalf("off mode must not fail: %v", err)
	}
	for _, cmd := range f.Commands[n:] {
		if strings.Contains(cmd, "verify") {
			t.Errorf("off mode must not verify: %s", cmd)
		}
	}
}

func TestPutUnitVerifiedWarn(t *testing.T) {
	c, f := newVerifyClient(VerifyWarn)
	w, err := c.PutUnitVerified(context.Background(), "a.service", brokenUnit, nil, nil)
	if err != nil {
		t.Fatalf("warn mode must not fail: %v", err)
	}
	if len(w) == 0 {
		t.Fatal("expected a warning for a broken unit")
	}
	if _, err := f.ReadUnit("a.service"); err != nil {
		t.Errorf("file should stay written in warn mode: %v", err)
	}
	// valid unit: no warning
	w, err = c.PutUnitVerified(context.Background(), "b.service", goodUnit, nil, nil)
	if err != nil || len(w) != 0 {
		t.Errorf("valid unit should be silent: w=%v err=%v", w, err)
	}
}

func TestPutUnitVerifiedErrorRollback(t *testing.T) {
	c, f := newVerifyClient(VerifyError)
	// brand new unit: rollback removes the file
	if _, err := c.PutUnitVerified(context.Background(), "a.service", brokenUnit, nil, nil); err == nil {
		t.Fatal("error mode must fail on a broken unit")
	}
	if _, err := f.ReadUnit("a.service"); err == nil {
		t.Error("broken new unit must be rolled back (removed)")
	}
	// existing unit: rollback restores previous content
	if err := f.WriteUnit("b.service", goodUnit); err != nil {
		t.Fatal(err)
	}
	if _, err := c.PutUnitVerified(context.Background(), "b.service", brokenUnit, nil, nil); err == nil {
		t.Fatal("error mode must fail on a broken update")
	}
	got, err := f.ReadUnit("b.service")
	if err != nil || got != goodUnit {
		t.Errorf("rollback must restore previous content, got %q err %v", got, err)
	}
}

func TestPutUnitVerifiedValid(t *testing.T) {
	c, f := newVerifyClient(VerifyError)
	w, err := c.PutUnitVerified(context.Background(), "c.service", goodUnit, accBool(true), accBool(false))
	if err != nil || len(w) != 0 {
		t.Fatalf("valid unit: w=%v err=%v", w, err)
	}
	joined := strings.Join(f.Commands, "\n")
	for _, want := range []string{"daemon-reload", "enable"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected %q in commands:\n%s", want, joined)
		}
	}
}

func TestSystemdVersion(t *testing.T) {
	c, _ := newVerifyClient(VerifyOff)
	info, err := c.systemdVersion()
	if err != nil {
		t.Fatalf("systemdVersion: %v", err)
	}
	if info.Version != "257" {
		t.Errorf("version = %q", info.Version)
	}
	if !strings.HasPrefix(info.Line, "systemd 257") {
		t.Errorf("line = %q", info.Line)
	}
}
