package remote

import "testing"

func TestNewNspawnDefaultMachine(t *testing.T) {
	n := NewNspawn("")
	if n.Machine != "tf-systemd-acc" {
		t.Fatalf("got %q", n.Machine)
	}
}

func TestNewNspawnCustomMachine(t *testing.T) {
	n := NewNspawn("my-machine")
	if n.Machine != "my-machine" {
		t.Fatalf("got %q", n.Machine)
	}
}

// TestNspawnImplementsHost is a compile-time-ish safety net in addition to the
// package-level `var _ Host = (*Nspawn)(nil)` check in nspawn.go.
func TestNspawnImplementsHost(t *testing.T) {
	var h Host = NewNspawn("")
	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestParseUnitStatus(t *testing.T) {
	out := "LoadState=loaded\nActiveState=active\nSubState=running\nUnitFileState=enabled\n"
	st := parseUnitStatus(out)
	want := UnitStatus{LoadState: "loaded", ActiveState: "active", SubState: "running", UnitFileState: "enabled"}
	if st != want {
		t.Fatalf("got %+v, want %+v", st, want)
	}
}

func TestParseLinkStatus(t *testing.T) {
	out := "● 2: eth0\n" +
		"       Link File: /usr/lib/systemd/network/99-default.link\n" +
		"    Network File: /etc/systemd/network/90-eth0.network\n" +
		"            State: routable (configured)\n" +
		"             Type: ether\n"
	st := parseLinkStatus("eth0", out)
	want := LinkStatus{Name: "eth0", OperationalState: "routable", SetupState: "configured"}
	if st != want {
		t.Fatalf("got %+v, want %+v", st, want)
	}
}
