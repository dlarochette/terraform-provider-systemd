package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUnitNameValidator(t *testing.T) {
	t.Parallel()
	v := unitNameValidator()
	cases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"ok service", "demo.service", false},
		{"ok timer", "backup.timer", false},
		{"path traversal", "../etc.service", true},
		{"slash", "a/b.service", true},
		{"bad suffix", "demo.txt", true},
		{"empty", "", true},
		{"dotdot", "..", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &validator.StringResponse{}
			v.ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("name"),
				ConfigValue: types.StringValue(tc.value),
			}, resp)
			hasErr := resp.Diagnostics.HasError()
			if hasErr != tc.wantErr {
				t.Fatalf("value %q: hasErr=%v want %v diags=%v", tc.value, hasErr, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}

func TestNetworkFilenameValidator(t *testing.T) {
	t.Parallel()
	v := networkFilenameValidator(".network")
	resp := &validator.StringResponse{}
	v.ValidateString(context.Background(), validator.StringRequest{
		Path:        path.Root("filename"),
		ConfigValue: types.StringValue("10-eth0.netdev"),
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected suffix mismatch error")
	}
	resp = &validator.StringResponse{}
	v.ValidateString(context.Background(), validator.StringRequest{
		Path:        path.Root("filename"),
		ConfigValue: types.StringValue("10-eth0.network"),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected: %v", resp.Diagnostics)
	}
}

func TestUnitSuffixValidator(t *testing.T) {
	t.Parallel()
	v := unitSuffixValidator(".timer")
	resp := &validator.StringResponse{}
	v.ValidateString(context.Background(), validator.StringRequest{
		ConfigValue: types.StringValue("backup.timer"),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	resp = &validator.StringResponse{}
	v.ValidateString(context.Background(), validator.StringRequest{
		ConfigValue: types.StringValue("backup.service"),
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected .timer required")
	}
}

func TestDropinNameValidator(t *testing.T) {
	t.Parallel()
	v := dropinNameValidator()
	resp := &validator.StringResponse{}
	v.ValidateString(context.Background(), validator.StringRequest{
		ConfigValue: types.StringValue("override.conf"),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	resp = &validator.StringResponse{}
	v.ValidateString(context.Background(), validator.StringRequest{
		ConfigValue: types.StringValue("override"),
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected .conf required")
	}
}
