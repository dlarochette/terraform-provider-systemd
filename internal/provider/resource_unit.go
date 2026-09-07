package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dlarochette/terraform-provider-systemd/internal/sdprops"
)

var _ resource.Resource = &unitResource{}
var _ resource.ResourceWithImportState = &unitResource{}
var _ resource.ResourceWithValidateConfig = &unitResource{}

func NewUnitResource() resource.Resource { return &unitResource{} }

type unitResource struct {
	client *Client
}

type unitModel struct {
	Name     types.String   `tfsdk:"name"`
	Content  types.String   `tfsdk:"content"`
	Sections []sectionModel `tfsdk:"section"`
	Enable   types.Bool     `tfsdk:"enable"`
	Active   types.Bool     `tfsdk:"active"`
	ID       types.String   `tfsdk:"id"`
}

func (r *unitResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_unit"
}

func (r *unitResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a systemd unit file under `/etc/systemd/system/{name}` on a remote host over SSH.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Unit filename (e.g. `demo.service`). Forces replacement when changed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					unitNameValidator(),
				},
			},
			"content": optionalContentAttribute(),
			"enable": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether the unit should be enabled (`systemctl enable` / `disable`).",
			},
			"active": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether the unit should be started (`systemctl start` / `stop`). Start failure fails the apply.",
			},
			"id": idAttribute(),
		},
		Blocks: map[string]schema.Block{
			"section": sectionBlockSchema(),
		},
	}
	// Typed systemd properties, one block per unit section.
	for name, block := range typedBlocks(sdprops.UnitSections()) {
		resp.Schema.Blocks[name] = block
	}
}

func (r *unitResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	d, err := unitLikeFromRaw(req.Config.Raw)
	if err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
		return
	}
	content := types.StringNull()
	if d.HasContent {
		content = types.StringValue(d.Content)
	}
	if err := validateContentSectionsTyped(content, d.Sections, req.Config.Raw, sdprops.UnitSections()); err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
		return
	}
	if err := validateTypedConflicts(req.Config.Raw, d.Sections, sdprops.UnitSections()); err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
	}
}

func (r *unitResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *unitResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	d, err := unitLikeFromRaw(req.Plan.Raw)
	if err != nil {
		resp.Diagnostics.AddError("read plan", err.Error())
		return
	}
	content := types.StringNull()
	if d.HasContent {
		content = types.StringValue(d.Content)
	}
	body, err := resolveFileContentTyped(content, d.Sections, req.Plan.Raw, sdprops.UnitSections())
	if err != nil {
		resp.Diagnostics.AddError("resolve content", err.Error())
		return
	}
	warnings, err := r.client.PutUnitVerified(ctx, d.Name, body, d.Enable, d.Active)
	for _, w := range warnings {
		resp.Diagnostics.AddWarning("systemd verification", w)
	}
	if err != nil {
		resp.Diagnostics.AddError("create unit", err.Error())
		return
	}
	resp.State.Raw = setStateContent(req.Plan.Raw, body, d.Name)
}

func (r *unitResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	d, err := unitLikeFromRaw(req.State.Raw)
	if err != nil {
		resp.Diagnostics.AddError("read state", err.Error())
		return
	}
	content, err := r.client.GetUnit(ctx, d.Name)
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.State.Raw = setStateContent(req.State.Raw, content, d.Name)
}

func (r *unitResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	d, err := unitLikeFromRaw(req.Plan.Raw)
	if err != nil {
		resp.Diagnostics.AddError("read plan", err.Error())
		return
	}
	content := types.StringNull()
	if d.HasContent {
		content = types.StringValue(d.Content)
	}
	body, err := resolveFileContentTyped(content, d.Sections, req.Plan.Raw, sdprops.UnitSections())
	if err != nil {
		resp.Diagnostics.AddError("resolve content", err.Error())
		return
	}
	warnings, err := r.client.PutUnitVerified(ctx, d.Name, body, d.Enable, d.Active)
	for _, w := range warnings {
		resp.Diagnostics.AddWarning("systemd verification", w)
	}
	if err != nil {
		resp.Diagnostics.AddError("update unit", err.Error())
		return
	}
	resp.State.Raw = setStateContent(req.Plan.Raw, body, d.Name)
}

func (r *unitResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	d, err := unitLikeFromRaw(req.State.Raw)
	if err != nil {
		resp.Diagnostics.AddError("read state", err.Error())
		return
	}
	if err := r.client.DeleteUnit(ctx, d.Name); err != nil {
		resp.Diagnostics.AddError("delete unit", err.Error())
	}
}

func (r *unitResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
