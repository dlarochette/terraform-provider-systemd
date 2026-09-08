package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dlarochette/terraform-provider-systemd/internal/sdprops"
)

var _ resource.Resource = &dropinResource{}
var _ resource.ResourceWithImportState = &dropinResource{}
var _ resource.ResourceWithValidateConfig = &dropinResource{}

func NewDropinResource() resource.Resource {
	specs := unitSpecs(sdprops.UnitSections())
	for i := range specs {
		if specs[i].Section == "Unit" {
			// `unit` is the resource attribute holding the parent unit;
			// rename the typed [Unit] block to avoid the collision.
			specs[i].Block = "unit_section"
		}
	}
	return &dropinResource{specs: specs}
}

type dropinResource struct {
	client *Client
	specs  []sectionSpec
}

type dropinModel struct {
	Unit     types.String   `tfsdk:"unit"`
	Dropin   types.String   `tfsdk:"dropin"`
	Content  types.String   `tfsdk:"content"`
	Sections []sectionModel `tfsdk:"section"`
	ID       types.String   `tfsdk:"id"`
}

func (r *dropinResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dropin"
}

func (r *dropinResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a systemd drop-in under `/etc/systemd/system/{unit}.d/{dropin}` over SSH.",
		Attributes: map[string]schema.Attribute{
			"unit": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Parent unit filename (e.g. `sshd.service`). Forces replacement when changed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					unitNameValidator(),
				},
			},
			"dropin": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Drop-in filename ending in `.conf` (e.g. `override.conf`). Forces replacement when changed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					dropinNameValidator(),
				},
			},
			"content": optionalContentAttribute(),
			"id":      idAttribute(),
		},
		Blocks: map[string]schema.Block{
			"section": sectionBlockSchema(),
		},
	}
	for name, block := range typedBlocks(r.specs) {
		resp.Schema.Blocks[name] = block
	}
}

func (r *dropinResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	d, err := dropinFromRaw(req.Config.Raw)
	if err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
		return
	}
	content := types.StringNull()
	if d.HasContent {
		content = types.StringValue(d.Content)
	}
	if err := validateContentSectionsTyped(content, d.Sections, req.Config.Raw, r.specs); err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
		return
	}
	if err := validateTypedConflicts(req.Config.Raw, d.Sections, r.specs); err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
	}
}

func (r *dropinResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, err := clientFrom(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("configure", err.Error())
		return
	}
	r.client = c
}

func (r *dropinResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	d, err := dropinFromRaw(req.Plan.Raw)
	if err != nil {
		resp.Diagnostics.AddError("read plan", err.Error())
		return
	}
	content := types.StringNull()
	if d.HasContent {
		content = types.StringValue(d.Content)
	}
	body, err := resolveFileContentTyped(content, d.Sections, req.Plan.Raw, r.specs)
	if err != nil {
		resp.Diagnostics.AddError("resolve content", err.Error())
		return
	}
	if err := r.client.PutDropin(ctx, d.Unit, d.Dropin, body); err != nil {
		resp.Diagnostics.AddError("create dropin", err.Error())
		return
	}
	resp.State.Raw = setStateContent(req.Plan.Raw, body, d.Unit+"/"+d.Dropin)
}

func (r *dropinResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	d, err := dropinFromRaw(req.State.Raw)
	if err != nil {
		resp.Diagnostics.AddError("read state", err.Error())
		return
	}
	content, err := r.client.GetDropin(ctx, d.Unit, d.Dropin)
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.State.Raw = setStateContent(req.State.Raw, content, d.Unit+"/"+d.Dropin)
}

func (r *dropinResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	d, err := dropinFromRaw(req.Plan.Raw)
	if err != nil {
		resp.Diagnostics.AddError("read plan", err.Error())
		return
	}
	content := types.StringNull()
	if d.HasContent {
		content = types.StringValue(d.Content)
	}
	body, err := resolveFileContentTyped(content, d.Sections, req.Plan.Raw, r.specs)
	if err != nil {
		resp.Diagnostics.AddError("resolve content", err.Error())
		return
	}
	if err := r.client.PutDropin(ctx, d.Unit, d.Dropin, body); err != nil {
		resp.Diagnostics.AddError("update dropin", err.Error())
		return
	}
	resp.State.Raw = setStateContent(req.Plan.Raw, body, d.Unit+"/"+d.Dropin)
}

func (r *dropinResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	d, err := dropinFromRaw(req.State.Raw)
	if err != nil {
		resp.Diagnostics.AddError("read state", err.Error())
		return
	}
	if err := r.client.DeleteDropin(ctx, d.Unit, d.Dropin); err != nil {
		resp.Diagnostics.AddError("delete dropin", err.Error())
	}
}

func (r *dropinResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("import", "expected id format unit/dropin.conf")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("unit"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("dropin"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
