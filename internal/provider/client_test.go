package provider

import (
	"strings"
	"testing"

	"github.com/toxn/terraform-provider-systemd/internal/remote"
)

func TestClientUnitDropinNetwork(t *testing.T) {
	h := remote.NewFake()
	c := &Client{Host: h}

	enable, active := true, false
	if err := c.PutUnit(t.Context(), "demo.service", "[Unit]\nDescription=x\n", &enable, &active); err != nil {
		t.Fatal(err)
	}
	if got, err := c.GetUnit(t.Context(), "demo.service"); err != nil || !strings.Contains(got, "Description=x") {
		t.Fatalf("get unit: %v %q", err, got)
	}

	disable := false
	if err := c.PutUnit(t.Context(), "demo.service", "[Unit]\nDescription=y\n", &disable, nil); err != nil {
		t.Fatal(err)
	}

	if err := c.PutDropin(t.Context(), "demo.service", "override.conf", "[Service]\nEnvironment=A=1\n"); err != nil {
		t.Fatal(err)
	}
	if got, err := c.GetDropin(t.Context(), "demo.service", "override.conf"); err != nil || !strings.Contains(got, "Environment") {
		t.Fatalf("dropin: %v %q", err, got)
	}

	if err := c.PutNetwork(t.Context(), "10-eth0.network", "[Match]\nName=eth0\n"); err != nil {
		t.Fatal(err)
	}
	if got, err := c.GetNetwork(t.Context(), "10-eth0.network"); err != nil || !strings.Contains(got, "eth0") {
		t.Fatalf("network: %v %q", err, got)
	}

	st, err := c.UnitStatus(t.Context(), "demo.service")
	if err != nil || st.ActiveState == "" {
		t.Fatalf("status: %v %+v", err, st)
	}
	link, err := c.LinkStatus(t.Context(), "eth0")
	if err != nil || link.Name != "eth0" {
		t.Fatalf("link: %v %+v", err, link)
	}

	if err := c.DeleteDropin(t.Context(), "demo.service", "override.conf"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteNetwork(t.Context(), "10-eth0.network"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteUnit(t.Context(), "demo.service"); err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(h.Commands, "|")
	for _, want := range []string{"daemon-reload", "enable", "disable", "networkctl reload"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %s", want, joined)
		}
	}
}

func TestClientStartFailure(t *testing.T) {
	h := remote.NewFake()
	h.FailCmd = "systemctl start"
	c := &Client{Host: h}
	active := true
	err := c.PutUnit(t.Context(), "bad.service", "[Unit]\nDescription=x\n", nil, &active)
	if err == nil {
		t.Fatal("expected start failure")
	}
}
