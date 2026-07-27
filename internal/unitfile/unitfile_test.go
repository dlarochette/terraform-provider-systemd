package unitfile

import (
	"strings"
	"testing"
)

func TestRenderParseRoundTrip(t *testing.T) {
	in := File{Sections: []Section{
		{Name: "Unit", Entries: []Entry{
			{Key: "Description", Value: "Demo"},
			{Key: "After", Value: "network.target"},
		}},
		{Name: "Service", Entries: []Entry{
			{Key: "Type", Value: "oneshot"},
			{Key: "ExecStart", Value: "/bin/true"},
			{Key: "ExecStart", Value: "/bin/false"},
		}},
		{Name: "Install", Entries: []Entry{
			{Key: "WantedBy", Value: "multi-user.target"},
		}},
	}}
	text := in.Render()
	out, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	if out.Render() != text {
		t.Fatalf("round-trip mismatch\nwant:\n%s\ngot:\n%s", text, out.Render())
	}
}

func TestParseNetwork(t *testing.T) {
	raw := `[Match]
Name=eth0

[Network]
Address=192.0.2.10/24
Gateway=192.0.2.1
`
	f, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Sections) != 2 || f.Sections[0].Name != "Match" {
		t.Fatalf("unexpected sections: %+v", f.Sections)
	}
	rendered := f.Render()
	if !strings.Contains(rendered, "Address=192.0.2.10/24") {
		t.Fatalf("missing address: %s", rendered)
	}
}

func TestChooseContentExclusive(t *testing.T) {
	_, err := ChooseContent("x=1\n", File{Sections: []Section{{Name: "Unit"}}})
	if err == nil {
		t.Fatal("expected exclusive error")
	}
	_, err = ChooseContent("", File{})
	if err == nil {
		t.Fatal("expected empty error")
	}
	got, err := ChooseContent("[Unit]\nDescription=x\n", File{})
	if err != nil || !strings.Contains(got, "Description=x") {
		t.Fatalf("raw: %v %q", err, got)
	}
}
