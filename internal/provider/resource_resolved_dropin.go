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

func NewResolvedDropinResource() resource.Resource { return &resolvedDropinResource{} }

type resolvedDropinResource struct{ client *Client }

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
}

func (r *resolvedDropinResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg resolvedDropinModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := validateContentOrSections(cfg.Content, cfg.Sections); err != nil {
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
	var plan resolvedDropinModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, err := resolveFileContent(plan.Content, plan.Sections)
	if err != nil {
		resp.Diagnostics.AddError("resolve content", err.Error())
		return
	}
	if err := r.client.PutResolvedDropin(ctx, plan.Name.ValueString(), body); err != nil {
		resp.Diagnostics.AddError("create resolved drop-in", err.Error())
		return
	}
	plan.Content = types.StringValue(body)
	plan.ID = plan.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *resolvedDropinResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resolvedDropinModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	content, err := r.client.GetResolvedDropin(ctx, state.Name.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.Content = types.StringValue(content)
	state.ID = state.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *resolvedDropinResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan resolvedDropinModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, err := resolveFileContent(plan.Content, plan.Sections)
	if err != nil {
		resp.Diagnostics.AddError("resolve content", err.Error())
		return
	}
	if err := r.client.PutResolvedDropin(ctx, plan.Name.ValueString(), body); err != nil {
		resp.Diagnostics.AddError("update resolved drop-in", err.Error())
		return
	}
	plan.Content = types.StringValue(body)
	plan.ID = plan.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *resolvedDropinResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resolvedDropinModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteResolvedDropin(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("delete resolved drop-in", err.Error())
	}
}

func (r *resolvedDropinResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
