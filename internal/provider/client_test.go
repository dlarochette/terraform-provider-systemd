package provider

import (
	"strings"
	"testing"

	"github.com/dlarochette/terraform-provider-systemd/internal/remote"
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

func TestClientResolved(t *testing.T) {
	h := remote.NewFake()
	c := &Client{Host: h}
	ctx := t.Context()

	if err := c.PutResolvedConf(ctx, "[Resolve]\nDNS=8.8.8.8\n"); err != nil {
		t.Fatal(err)
	}
	if got, err := c.GetResolvedConf(ctx); err != nil || !strings.Contains(got, "8.8.8.8") {
		t.Fatalf("%v %q", err, got)
	}
	if err := c.PutResolvedDropin(ctx, "10-x.conf", "[Resolve]\nDNS=1.1.1.1\n"); err != nil {
		t.Fatal(err)
	}
	route := true
	if err := c.PutResolveLink(ctx, "eth0", ResolveLinkDesired{
		DNS:          []string{"1.1.1.1"},
		Domains:      []string{"~lan"},
		DefaultRoute: &route,
		LLMNR:        "no",
	}); err != nil {
		t.Fatal(err)
	}
	st, err := c.ResolveStatus(ctx, "eth0")
	if err != nil || !strings.Contains(st, "1.1.1.1") {
		t.Fatalf("%v %q", err, st)
	}
	if err := c.DeleteResolveLink(ctx, "eth0"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteResolvedDropin(ctx, "10-x.conf"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteResolvedConf(ctx); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(h.Commands, "|")
	if !strings.Contains(joined, "systemd-resolved.service") || !strings.Contains(joined, "resolvectl revert") {
		t.Fatalf("commands: %s", joined)
	}
}

func TestClientApplyAndDeleteUnitLifecycle(t *testing.T) {
	h := remote.NewFake()
	c := &Client{Host: h}

	enable, active := true, true
	if err := c.ApplyUnitLifecycle(t.Context(), "app@bar.service", &enable, &active); err != nil {
		t.Fatal(err)
	}
	c.DeleteUnitLifecycle(t.Context(), "app@bar.service")

	joined := strings.Join(h.Commands, "|")
	for _, want := range []string{"enable app@bar.service", "start app@bar.service", "stop app@bar.service", "disable app@bar.service"} {
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
