//go:build !skip_acc

package provider

import (
	"strings"
	"testing"
	"time"
)

func TestAccResolvedDropin(t *testing.T) {
	c := accClient(t)
	ctx := t.Context()

	if err := c.ApplyUnitLifecycle(ctx, "systemd-resolved.service", accBool(true), accBool(true)); err != nil {
		t.Fatalf("start systemd-resolved: %v", err)
	}

	const name = "90-tf-acc.conf"
	const content = "[Resolve]\n" +
		"DNS=9.9.9.9\n"

	if err := c.PutResolvedDropin(ctx, name, content); err != nil {
		t.Fatalf("PutResolvedDropin: %v", err)
	}
	t.Cleanup(func() {
		if err := c.DeleteResolvedDropin(ctx, name); err != nil {
			t.Logf("cleanup DeleteResolvedDropin: %v", err)
		}
	})

	got, err := c.GetResolvedDropin(ctx, name)
	if err != nil {
		t.Fatalf("GetResolvedDropin: %v", err)
	}
	if !strings.Contains(got, "DNS=9.9.9.9") {
		t.Fatalf("unexpected drop-in content: %q", got)
	}

	st, err := c.ResolveStatus(ctx, "")
	if err != nil {
		t.Fatalf("ResolveStatus: %v", err)
	}
	if strings.TrimSpace(st) == "" {
		t.Fatal("empty resolvectl status")
	}
}

func TestAccResolvedConf(t *testing.T) {
	c := accClient(t)
	ctx := t.Context()

	if err := c.ApplyUnitLifecycle(ctx, "systemd-resolved.service", accBool(true), accBool(true)); err != nil {
		t.Fatalf("start systemd-resolved: %v", err)
	}

	const content = "[Resolve]\n" +
		"FallbackDNS=8.8.8.8\n"

	prev, _ := c.GetResolvedConf(ctx)
	t.Cleanup(func() {
		if prev != "" {
			_ = c.PutResolvedConf(ctx, prev)
		} else {
			_ = c.DeleteResolvedConf(ctx)
		}
	})

	if err := c.PutResolvedConf(ctx, content); err != nil {
		t.Fatalf("PutResolvedConf: %v", err)
	}
	got, err := c.GetResolvedConf(ctx)
	if err != nil {
		t.Fatalf("GetResolvedConf: %v", err)
	}
	if !strings.Contains(got, "FallbackDNS=8.8.8.8") {
		t.Fatalf("unexpected resolved.conf: %q", got)
	}
}

func TestAccResolveLink(t *testing.T) {
	c := accClient(t)
	ctx := t.Context()

	if err := c.ApplyUnitLifecycle(ctx, "systemd-resolved.service", accBool(true), accBool(true)); err != nil {
		t.Fatalf("start systemd-resolved: %v", err)
	}

	const link = "tfaccdn0"
	_ = c.Host.Exec("ip", "link", "del", link)
	if err := c.Host.Exec("ip", "link", "add", link, "type", "dummy"); err != nil {
		t.Fatalf("ip link add: %v", err)
	}
	t.Cleanup(func() {
		_ = c.DeleteResolveLink(ctx, link)
		_ = c.Host.Exec("ip", "link", "del", link)
	})
	if err := c.Host.Exec("ip", "link", "set", link, "up"); err != nil {
		t.Fatalf("ip link set up: %v", err)
	}

	desired := ResolveLinkDesired{
		DNS:     []string{"1.1.1.1", "1.0.0.1"},
		Domains: []string{"~tf-acc.test"},
	}
	if err := c.PutResolveLink(ctx, link, desired); err != nil {
		t.Fatalf("PutResolveLink: %v", err)
	}

	// resolvectl may race the apply (NUL-byte output on the SSH transport
	// occasionally); retry for up to ~5s before giving up.
	var dns []string
	var err error
	deadline := time.Now().Add(5 * time.Second)
	for {
		dns, err = c.GetResolveLinkDNS(ctx, link)
		if err != nil {
			t.Fatalf("GetResolveLinkDNS: %v", err)
		}
		if strings.Contains(strings.Join(dns, " "), "1.1.1.1") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("dns not applied: %v", dns)
		}
		time.Sleep(250 * time.Millisecond)
	}

	st, err := c.ResolveStatus(ctx, link)
	if err != nil {
		t.Fatalf("ResolveStatus link: %v", err)
	}
	if !strings.Contains(st, link) && !strings.Contains(st, "1.1.1.1") {
		t.Fatalf("unexpected status: %q", st)
	}

	if err := c.DeleteResolveLink(ctx, link); err != nil {
		t.Fatalf("DeleteResolveLink: %v", err)
	}
}
