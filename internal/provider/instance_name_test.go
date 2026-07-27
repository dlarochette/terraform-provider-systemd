package provider

import "testing"

func TestInstantiateUnit(t *testing.T) {
	got, err := InstantiateUnit("app@.service", "bar")
	if err != nil || got != "app@bar.service" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestParseInstantiatedUnit(t *testing.T) {
	tpl, inst, err := ParseInstantiatedUnit("app@bar.service")
	if err != nil || tpl != "app@.service" || inst != "bar" {
		t.Fatalf("%q %q %v", tpl, inst, err)
	}
	if _, _, err := ParseInstantiatedUnit("app@.service"); err == nil {
		t.Fatal("expected error for template-only name")
	}
}
