package remote

import (
	"os/exec"
	"testing"
)

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

func TestExitCode(t *testing.T) {
	if got := exitCode(nil); got != -1 {
		t.Fatalf("nil err: got %d, want -1", got)
	}

	// LookPath failure: not an *exec.ExitError, must not be mistaken for exit code 1.
	notFoundErr := exec.Command("definitely-not-a-real-binary-xyz").Run()
	if got := exitCode(notFoundErr); got != -1 {
		t.Fatalf("lookup err: got %d, want -1", got)
	}

	exit1Err := exec.Command("sh", "-c", "exit 1").Run()
	if got := exitCode(exit1Err); got != 1 {
		t.Fatalf("exit 1: got %d, want 1", got)
	}

	exit3Err := exec.Command("sh", "-c", "exit 3").Run()
	if got := exitCode(exit3Err); got != 3 {
		t.Fatalf("exit 3: got %d, want 3", got)
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
