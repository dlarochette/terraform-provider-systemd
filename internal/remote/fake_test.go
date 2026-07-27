package remote

import (
	"strings"
	"testing"
)

func TestFakeUnitLifecycle(t *testing.T) {
	h := NewFake()
	content := "[Unit]\nDescription=demo\n"
	if err := h.WriteUnit("demo.service", content); err != nil {
		t.Fatal(err)
	}
	got, err := h.ReadUnit("demo.service")
	if err != nil || got != content {
		t.Fatalf("read: %v %q", err, got)
	}
	if err := h.DaemonReload(); err != nil {
		t.Fatal(err)
	}
	if err := h.EnableUnit("demo.service"); err != nil {
		t.Fatal(err)
	}
	if err := h.StartUnit("demo.service"); err != nil {
		t.Fatal(err)
	}
	if err := h.RemoveUnit("demo.service"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.ReadUnit("demo.service"); err == nil {
		t.Fatal("expected missing")
	}
	joined := strings.Join(h.Commands, ",")
	if !strings.Contains(joined, "daemon-reload") || !strings.Contains(joined, "enable") {
		t.Fatalf("commands: %v", h.Commands)
	}
}

func TestFakeDropinAndNetwork(t *testing.T) {
	h := NewFake()
	if err := h.WriteDropin("a.service", "10-x.conf", "[Service]\nNice=1\n"); err != nil {
		t.Fatal(err)
	}
	got, err := h.ReadDropin("a.service", "10-x.conf")
	if err != nil || !strings.Contains(got, "Nice=1") {
		t.Fatalf("%v %q", err, got)
	}
	if err := h.WriteNetwork("x.network", "[Network]\nDHCP=yes\n"); err != nil {
		t.Fatal(err)
	}
	if err := h.NetworkReload(); err != nil {
		t.Fatal(err)
	}
	if err := h.RemoveDropin("a.service", "10-x.conf"); err != nil {
		t.Fatal(err)
	}
	if err := h.RemoveNetwork("x.network"); err != nil {
		t.Fatal(err)
	}
}

func TestSafeNameRejectsTraversal(t *testing.T) {
	if _, err := unitPath(DefaultUnitDir, "../passwd"); err == nil {
		t.Fatal("expected error")
	}
}

func TestPathHelpersReject(t *testing.T) {
	cases := []string{"", "../x", "a/b", ".", ".."}
	for _, name := range cases {
		if _, err := unitPath(DefaultUnitDir, name); err == nil {
			t.Fatalf("expected reject %q", name)
		}
		if _, err := networkPath(DefaultNetworkDir, name); err == nil {
			t.Fatalf("expected reject network %q", name)
		}
	}
	if _, err := dropinPath(DefaultUnitDir, "ok.service", "../x.conf"); err == nil {
		t.Fatal("expected dropin reject")
	}
}

func TestStartFailure(t *testing.T) {
	h := NewFake()
	h.FailCmd = "systemctl start"
	if err := h.StartUnit("x.service"); err == nil {
		t.Fatal("expected fail")
	}
}
