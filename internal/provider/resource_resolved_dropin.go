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
)

var _ resource.Resource = &resolvedDropinResource{}
var _ resource.ResourceWithImportState = &resolvedDropinResource{}
var _ resource.ResourceWithValidateConfig = &resolvedDropinResource{}

func NewResolvedDropinResource() resource.Resource {
	return &resolvedDropinResource{specs: netSpecs("resolved")}
}

type resolvedDropinResource struct {
	client *Client
	specs  []sectionSpec
}

type resolvedDropinModel struct {
	Name     types.String   `tfsdk:"name"`
	Content  types.String   `tfsdk:"content"`
	Sections []sectionModel `tfsdk:"section"`
	ID       types.String   `tfsdk:"id"`
}

func (r *resolvedDropinResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resolved_dropin"
}

func (r *resolvedDropinResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a drop-in under `/etc/systemd/resolved.conf.d/{name}` over SSH. " +
			"Apply and destroy restart `systemd-resolved.service`.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Drop-in filename ending in `.conf`. Forces replacement when changed.",
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

func (r *resolvedDropinResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	d, err := unitLikeFromRaw(req.Config.Raw)
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

func (r *resolvedDropinResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *resolvedDropinResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	d, err := unitLikeFromRaw(req.Plan.Raw)
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
	if err := r.client.PutResolvedDropin(ctx, d.Name, body); err != nil {
		resp.Diagnostics.AddError("create resolved drop-in", err.Error())
		return
	}
	resp.State.Raw = setStateContent(req.Plan.Raw, body, d.Name)
}

func (r *resolvedDropinResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	d, err := unitLikeFromRaw(req.State.Raw)
	if err != nil {
		resp.Diagnostics.AddError("read state", err.Error())
		return
	}
	content, err := r.client.GetResolvedDropin(ctx, d.Name)
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.State.Raw = setStateContent(req.State.Raw, content, d.Name)
}

func (r *resolvedDropinResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	d, err := unitLikeFromRaw(req.Plan.Raw)
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
	if err := r.client.PutResolvedDropin(ctx, d.Name, body); err != nil {
		resp.Diagnostics.AddError("update resolved drop-in", err.Error())
		return
	}
	resp.State.Raw = setStateContent(req.Plan.Raw, body, d.Name)
}

func (r *resolvedDropinResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	d, err := unitLikeFromRaw(req.State.Raw)
	if err != nil {
		resp.Diagnostics.AddError("read state", err.Error())
		return
	}
	if err := r.client.DeleteResolvedDropin(ctx, d.Name); err != nil {
		resp.Diagnostics.AddError("delete resolved drop-in", err.Error())
	}
}

func (r *resolvedDropinResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
