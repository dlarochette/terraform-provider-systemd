package remote

import "testing"

func mustType(t *testing.T, s string) MachineImageType {
	t.Helper()
	ty, err := ParseMachineImageType(s)
	if err != nil {
		t.Fatal(err)
	}
	return ty
}

func TestMachinectlEnsureArgs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		typ, source string
		want        []string
	}{
		{"tar", "https://example.com/a.tar", []string{"pull-tar", "https://example.com/a.tar", "web"}},
		{"raw", "https://example.com/a.raw", []string{"pull-raw", "https://example.com/a.raw", "web"}},
		{"oci", "docker.io/library/debian:bookworm", []string{"pull-dkr", "docker.io/library/debian:bookworm", "web"}},
		{"local", "/var/tmp/mini.tar", []string{"import-tar", "/var/tmp/mini.tar", "web"}},
		{"local", "/var/tmp/mini.raw", []string{"import-raw", "/var/tmp/mini.raw", "web"}},
	}
	for _, tc := range cases {
		got, err := MachinectlEnsureArgs("web", mustType(t, tc.typ), tc.source)
		if err != nil {
			t.Fatalf("%s: %v", tc.typ, err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("%s: got %#v want %#v", tc.typ, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%s: got %#v want %#v", tc.typ, got, tc.want)
			}
		}
	}
}

func TestMachinectlEnsureArgsLocalDir(t *testing.T) {
	t.Parallel()
	_, err := MachinectlEnsureArgs("web", MachineImageLocal, "/var/tmp/rootfs")
	if err == nil {
		t.Fatal("expected error for local directory path")
	}
}

func TestNspawnUnitName(t *testing.T) {
	t.Parallel()
	if got := NspawnUnitName("web"); got != "systemd-nspawn@web.service" {
		t.Fatalf("got %q", got)
	}
}

func TestParseMachineImageType(t *testing.T) {
	t.Parallel()
	if _, err := ParseMachineImageType("nope"); err == nil {
		t.Fatal("expected error")
	}
}
