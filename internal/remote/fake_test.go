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

func TestFakeResolved(t *testing.T) {
	h := NewFake()
	if err := h.Exec("true"); err != nil {
		t.Fatal(err)
	}
	body := "[Resolve]\nDNS=9.9.9.9\n"
	if err := h.WriteResolvedConf(body); err != nil {
		t.Fatal(err)
	}
	got, err := h.ReadResolvedConf()
	if err != nil || got != body {
		t.Fatalf("%v %q", err, got)
	}
	if err := h.WriteResolvedDropin("10-tf.conf", "[Resolve]\nDNS=1.1.1.1\n"); err != nil {
		t.Fatal(err)
	}
	if err := h.ResolvedRestart(); err != nil {
		t.Fatal(err)
	}
	if err := h.ResolvectlDNS("eth0", []string{"1.1.1.1", "1.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	dns, err := h.ResolvectlDNSGet("eth0")
	if err != nil || len(dns) != 2 || dns[0] != "1.1.1.1" {
		t.Fatalf("%v %v", err, dns)
	}
	st, err := h.ResolvectlStatus("eth0")
	if err != nil || !strings.Contains(st, "1.1.1.1") {
		t.Fatalf("%v %q", err, st)
	}
	if err := h.ResolvectlRevert("eth0"); err != nil {
		t.Fatal(err)
	}
	if err := h.RemoveResolvedDropin("10-tf.conf"); err != nil {
		t.Fatal(err)
	}
	if err := h.RemoveResolvedConf(); err != nil {
		t.Fatal(err)
	}
}

func TestParseResolvectlList(t *testing.T) {
	got := parseResolvectlList("Link 2 (eth0): 1.1.1.1 1.0.0.1\n")
	if len(got) != 2 || got[0] != "1.1.1.1" {
		t.Fatalf("%v", got)
	}
}

func TestFakeMachine(t *testing.T) {
	h := NewFake()
	if err := h.EnsureMachineImage("web", "local", "/tmp/a.tar"); err != nil {
		t.Fatal(err)
	}
	st, err := h.ShowMachine("web")
	if err != nil || !st.ImagePresent {
		t.Fatalf("%+v %v", st, err)
	}
	body := "[Exec]\nBoot=no\n"
	if err := h.WriteNspawnFile("web", body); err != nil {
		t.Fatal(err)
	}
	got, err := h.ReadNspawnFile("web")
	if err != nil || got != body {
		t.Fatalf("%v %q", err, got)
	}
	h.Statuses[NspawnUnitName("web")] = UnitStatus{ActiveState: "active", LoadState: "loaded"}
	st, err = h.ShowMachine("web")
	if err != nil || st.Unit.ActiveState != "active" || st.Settings != body {
		t.Fatalf("%+v %v", st, err)
	}
	if err := h.RemoveMachineImage("web"); err != nil {
		t.Fatal(err)
	}
	st, _ = h.ShowMachine("web")
	if st.ImagePresent {
		t.Fatal("expected image gone")
	}
}
