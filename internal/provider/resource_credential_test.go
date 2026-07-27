package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dlarochette/terraform-provider-systemd/internal/remote"
)

func TestCredentialResourceCRUD(t *testing.T) {
	ctx := t.Context()
	h := remote.NewFake()
	client := &Client{Host: h}

	r := &credentialResource{client: client}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema: %+v", schemaResp.Diagnostics)
	}

	plan := credentialModel{
		Name:      types.StringValue("db-pass"),
		Data:      types.StringValue("s3cr3t"),
		Encrypted: types.BoolValue(true),
		WithKey:   types.StringValue("host"),
		ID:        types.StringUnknown(),
	}
	planState := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := planState.Set(ctx, &plan); diags.HasError() {
		t.Fatalf("set plan: %+v", diags)
	}

	createResp := &resource.CreateResponse{
		State: emptyInstanceState(t, schemaResp.Schema),
	}
	r.Create(ctx, resource.CreateRequest{Plan: planState}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create: %+v", createResp.Diagnostics)
	}

	var created credentialModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() {
		t.Fatalf("get created state: %+v", diags)
	}
	if created.ID.ValueString() != "db-pass" {
		t.Fatalf("expected id db-pass, got %q", created.ID.ValueString())
	}

	joined := strings.Join(h.Commands, "|")
	if !strings.Contains(joined, "systemd-creds encrypt --name=db-pass --with-key=host") {
		t.Fatalf("expected systemd-creds encrypt with --with-key=host, got %s", joined)
	}

	// Read: credential exists -> resource kept, data untouched (never refreshed from remote).
	readResp := &resource.ReadResponse{State: emptyInstanceState(t, schemaResp.Schema)}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read: %+v", readResp.Diagnostics)
	}
	var read credentialModel
	if diags := readResp.State.Get(ctx, &read); diags.HasError() {
		t.Fatalf("get read state: %+v", diags)
	}
	if read.Data.ValueString() != "s3cr3t" {
		t.Fatalf("expected data kept from state, got %q", read.Data.ValueString())
	}

	// Delete, then Read: credential gone -> resource removed.
	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: createResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete: %+v", deleteResp.Diagnostics)
	}

	readResp2 := &resource.ReadResponse{State: emptyInstanceState(t, schemaResp.Schema)}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp2)
	if readResp2.Diagnostics.HasError() {
		t.Fatalf("read (removed): %+v", readResp2.Diagnostics)
	}
	if !readResp2.State.Raw.IsNull() {
		t.Fatal("expected resource to be removed from state when credential no longer exists")
	}
}

func TestCredentialResourceDefaultEncrypted(t *testing.T) {
	ctx := t.Context()
	h := remote.NewFake()
	client := &Client{Host: h}

	r := &credentialResource{client: client}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema: %+v", schemaResp.Diagnostics)
	}

	plan := credentialModel{
		Name:      types.StringValue("db-pass"),
		Data:      types.StringValue("s3cr3t"),
		Encrypted: types.BoolNull(),
		WithKey:   types.StringNull(),
		ID:        types.StringUnknown(),
	}
	planState := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := planState.Set(ctx, &plan); diags.HasError() {
		t.Fatalf("set plan: %+v", diags)
	}
	// Simulate the framework applying the schema default for "encrypted"
	// before Create, since raw model.Set does not run plan modifiers/defaults.
	if diags := planState.SetAttribute(ctx, path.Root("encrypted"), true); diags.HasError() {
		t.Fatalf("set default encrypted: %+v", diags)
	}

	createResp := &resource.CreateResponse{
		State: emptyInstanceState(t, schemaResp.Schema),
	}
	r.Create(ctx, resource.CreateRequest{Plan: planState}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create: %+v", createResp.Diagnostics)
	}

	joined := strings.Join(h.Commands, "|")
	if !strings.Contains(joined, "systemd-creds encrypt --name=db-pass") {
		t.Fatalf("expected encrypted write by default, got %s", joined)
	}
}

func TestCredentialResourceValidateConfig(t *testing.T) {
	ctx := t.Context()
	r := &credentialResource{}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema: %+v", schemaResp.Diagnostics)
	}

	cfg := credentialModel{
		Name:      types.StringValue("db-pass"),
		Data:      types.StringValue("s3cr3t"),
		Encrypted: types.BoolValue(false),
		WithKey:   types.StringValue("host"),
		ID:        types.StringUnknown(),
	}
	config := configFromModel(t, schemaResp.Schema, &cfg)

	resp := &resource.ValidateConfigResponse{}
	r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for with_key set with encrypted = false")
	}
}

func TestCredentialResourceValidateConfigOK(t *testing.T) {
	ctx := t.Context()
	r := &credentialResource{}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema: %+v", schemaResp.Diagnostics)
	}

	cfg := credentialModel{
		Name:      types.StringValue("db-pass"),
		Data:      types.StringValue("s3cr3t"),
		Encrypted: types.BoolValue(true),
		WithKey:   types.StringValue("host"),
		ID:        types.StringUnknown(),
	}
	config := configFromModel(t, schemaResp.Schema, &cfg)

	resp := &resource.ValidateConfigResponse{}
	r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: config}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %+v", resp.Diagnostics)
	}
}

// configFromModel builds a tfsdk.Config from a Go model by round-tripping
// through a Plan, since tfsdk.Config (unlike Plan/State) has no Set method.
func configFromModel(t *testing.T, sch rschema.Schema, model interface{}) tfsdk.Config {
	t.Helper()
	plan := tfsdk.Plan{Schema: sch}
	if diags := plan.Set(t.Context(), model); diags.HasError() {
		t.Fatalf("set plan: %+v", diags)
	}
	return tfsdk.Config{Raw: plan.Raw, Schema: sch}
}
