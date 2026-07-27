package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/dlarochette/terraform-provider-systemd/internal/remote"
)

func emptyInstanceState(t *testing.T, sch rschema.Schema) tfsdk.State {
	t.Helper()
	return tfsdk.State{
		Raw:    tftypes.NewValue(sch.Type().TerraformType(t.Context()), nil),
		Schema: sch,
	}
}

func TestInstanceResourceCRUD(t *testing.T) {
	ctx := t.Context()
	h := remote.NewFake()
	client := &Client{Host: h}

	r := &instanceResource{client: client}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema: %+v", schemaResp.Diagnostics)
	}

	plan := instanceModel{
		Template: types.StringValue("app@.service"),
		Instance: types.StringValue("bar"),
		Enable:   types.BoolValue(true),
		Active:   types.BoolValue(true),
		ID:       types.StringUnknown(),
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

	var created instanceModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() {
		t.Fatalf("get created state: %+v", diags)
	}
	if created.ID.ValueString() != "app@bar.service" {
		t.Fatalf("expected id app@bar.service, got %q", created.ID.ValueString())
	}

	joined := strings.Join(h.Commands, "|")
	for _, want := range []string{"enable app@bar.service", "start app@bar.service"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %s", want, joined)
		}
	}

	// Read: unit exists (fake defaults to loaded/inactive) -> resource kept.
	readResp := &resource.ReadResponse{State: emptyInstanceState(t, schemaResp.Schema)}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read: %+v", readResp.Diagnostics)
	}
	if readResp.State.Raw.IsNull() {
		t.Fatal("expected resource to remain in state after read")
	}

	// Read: unit not found -> resource removed.
	h.Statuses["app@bar.service"] = remote.UnitStatus{LoadState: "not-found", ActiveState: "inactive"}
	readResp2 := &resource.ReadResponse{State: emptyInstanceState(t, schemaResp.Schema)}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp2)
	if readResp2.Diagnostics.HasError() {
		t.Fatalf("read (not-found): %+v", readResp2.Diagnostics)
	}
	if !readResp2.State.Raw.IsNull() {
		t.Fatal("expected resource to be removed from state when unit not found")
	}
	delete(h.Statuses, "app@bar.service")

	// Delete: best-effort stop/disable.
	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: createResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete: %+v", deleteResp.Diagnostics)
	}
	joined = strings.Join(h.Commands, "|")
	if !strings.Contains(joined, "stop app@bar.service") || !strings.Contains(joined, "disable app@bar.service") {
		t.Fatalf("expected stop+disable on delete, got %s", joined)
	}

	// ImportState: parses template/instance from the instantiated id.
	importResp := &resource.ImportStateResponse{State: emptyInstanceState(t, schemaResp.Schema)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "app@bar.service"}, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("import: %+v", importResp.Diagnostics)
	}
	var imported instanceModel
	if diags := importResp.State.Get(ctx, &imported); diags.HasError() {
		t.Fatalf("get imported state: %+v", diags)
	}
	if imported.Template.ValueString() != "app@.service" || imported.Instance.ValueString() != "bar" {
		t.Fatalf("unexpected import result: %+v", imported)
	}

	// ImportState: rejects non-instantiated names.
	badImportResp := &resource.ImportStateResponse{State: emptyInstanceState(t, schemaResp.Schema)}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "app@.service"}, badImportResp)
	if !badImportResp.Diagnostics.HasError() {
		t.Fatal("expected error importing a template-only name")
	}
}
