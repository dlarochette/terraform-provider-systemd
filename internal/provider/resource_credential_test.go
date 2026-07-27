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
		t.Fatal("expected data kept from state, got a different value")
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

// TestCredentialResourceEncryptedRequiresReplace documents that `encrypted`
// forces replacement instead of an in-place update. Without this, Terraform
// could switch a credential between /etc/credstore and
// /etc/credstore.encrypted via Update, which never removes the file at the
// old path and leaves an orphaned (potentially plaintext) copy behind.
func TestCredentialResourceEncryptedRequiresReplace(t *testing.T) {
	ctx := t.Context()
	r := &credentialResource{}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema: %+v", schemaResp.Diagnostics)
	}

	attr, ok := schemaResp.Schema.Attributes["encrypted"]
	if !ok {
		t.Fatal("schema has no \"encrypted\" attribute")
	}
	boolAttr, ok := attr.(rschema.BoolAttribute)
	if !ok {
		t.Fatalf("encrypted attribute is not a BoolAttribute: %T", attr)
	}
	if len(boolAttr.PlanModifiers) == 0 {
		t.Fatal("expected encrypted to have at least one plan modifier")
	}
	const wantDescription = "If the value of this attribute changes, Terraform will destroy and recreate the resource."
	found := false
	for _, m := range boolAttr.PlanModifiers {
		if m.Description(ctx) == wantDescription {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected encrypted to carry a RequiresReplace plan modifier")
	}
}

// TestCredentialResourceReplaceOnEncryptedChangeAvoidsOrphan simulates the
// destroy-then-create cycle Terraform performs when RequiresReplace forces
// replacement on an `encrypted` change, and asserts that Delete removes the
// credential at the path matching the old state (so nothing is orphaned)
// while Create writes only the new path.
func TestCredentialResourceReplaceOnEncryptedChangeAvoidsOrphan(t *testing.T) {
	ctx := t.Context()
	h := remote.NewFake()
	client := &Client{Host: h}

	r := &credentialResource{client: client}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema: %+v", schemaResp.Diagnostics)
	}

	encryptedPath := remote.DefaultCredstoreEncryptedDir + "/db-pass"
	plaintextPath := remote.DefaultCredstoreDir + "/db-pass"

	// Original resource: encrypted = true.
	oldPlan := credentialModel{
		Name:      types.StringValue("db-pass"),
		Data:      types.StringValue("s3cr3t"),
		Encrypted: types.BoolValue(true),
		WithKey:   types.StringNull(),
		ID:        types.StringUnknown(),
	}
	oldPlanState := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := oldPlanState.Set(ctx, &oldPlan); diags.HasError() {
		t.Fatalf("set old plan: %+v", diags)
	}
	oldCreateResp := &resource.CreateResponse{State: emptyInstanceState(t, schemaResp.Schema)}
	r.Create(ctx, resource.CreateRequest{Plan: oldPlanState}, oldCreateResp)
	if oldCreateResp.Diagnostics.HasError() {
		t.Fatalf("create old: %+v", oldCreateResp.Diagnostics)
	}
	if _, ok := h.Files[encryptedPath]; !ok {
		t.Fatalf("expected %s to exist after create", encryptedPath)
	}

	// Because encrypted has RequiresReplace, Terraform destroys the old
	// resource (state: encrypted = true) before creating the new one
	// (plan: encrypted = false), rather than calling Update.
	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: oldCreateResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete old: %+v", deleteResp.Diagnostics)
	}
	if _, ok := h.Files[encryptedPath]; ok {
		t.Fatalf("expected %s to be removed by destroy", encryptedPath)
	}

	newPlan := credentialModel{
		Name:      types.StringValue("db-pass"),
		Data:      types.StringValue("s3cr3t"),
		Encrypted: types.BoolValue(false),
		WithKey:   types.StringNull(),
		ID:        types.StringUnknown(),
	}
	newPlanState := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := newPlanState.Set(ctx, &newPlan); diags.HasError() {
		t.Fatalf("set new plan: %+v", diags)
	}
	newCreateResp := &resource.CreateResponse{State: emptyInstanceState(t, schemaResp.Schema)}
	r.Create(ctx, resource.CreateRequest{Plan: newPlanState}, newCreateResp)
	if newCreateResp.Diagnostics.HasError() {
		t.Fatalf("create new: %+v", newCreateResp.Diagnostics)
	}

	if _, ok := h.Files[plaintextPath]; !ok {
		t.Fatalf("expected %s to exist after replace", plaintextPath)
	}
	if _, ok := h.Files[encryptedPath]; ok {
		t.Fatalf("orphan detected: %s still present after replace", encryptedPath)
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
