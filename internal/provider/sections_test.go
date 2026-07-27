package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestResolveFileContentFromSections(t *testing.T) {
	sections := []sectionModel{
		{
			Name: types.StringValue("Unit"),
			Entries: []entryModel{
				{Key: types.StringValue("Description"), Value: types.StringValue("Demo")},
			},
		},
		{
			Name: types.StringValue("Service"),
			Entries: []entryModel{
				{Key: types.StringValue("Type"), Value: types.StringValue("oneshot")},
				{Key: types.StringValue("ExecStart"), Value: types.StringValue("/bin/true")},
			},
		},
	}
	got, err := resolveFileContent(types.StringNull(), sections)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[Unit]", "Description=Demo", "[Service]", "ExecStart=/bin/true"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestValidateContentOrSections(t *testing.T) {
	if err := validateContentOrSections(types.StringNull(), nil); err == nil {
		t.Fatal("expected empty error")
	}
	if err := validateContentOrSections(types.StringValue("x"), []sectionModel{{Name: types.StringValue("Unit")}}); err == nil {
		t.Fatal("expected exclusive error")
	}
	if err := validateContentOrSections(types.StringValue("[Unit]\n"), nil); err != nil {
		t.Fatal(err)
	}
	if err := validateContentOrSections(types.StringNull(), []sectionModel{{Name: types.StringValue("Unit")}}); err != nil {
		t.Fatal(err)
	}
}
