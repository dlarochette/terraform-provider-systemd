package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &resolvedResource{}
var _ resource.ResourceWithImportState = &resolvedResource{}
var _ resource.ResourceWithValidateConfig = &resolvedResource{}

func NewResolvedResource() resource.Resource { return &resolvedResource{} }

type resolvedResource struct{ client *Client }

type resolvedModel struct {
	Content  types.String   `tfsdk:"content"`
	Sections []sectionModel `tfsdk:"section"`
	ID       types.String   `tfsdk:"id"`
}

func (r *resolvedResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resolved"
}

func (r *resolvedResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages `/etc/systemd/resolved.conf` over SSH. Apply runs " +
			"`systemctl restart systemd-resolved.service`. Destroy removes the file and restarts " +
			"(vendor defaults apply again).",
		Attributes: map[string]schema.Attribute{
			"content": optionalContentAttribute(),
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Always `resolved.conf`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"section": sectionBlockSchema(),
		},
	}
}

func (r *resolvedResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg resolvedModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := validateContentOrSections(cfg.Content, cfg.Sections); err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
	}
}

func (r *resolvedResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *resolvedResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resolvedModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, err := resolveFileContent(plan.Content, plan.Sections)
	if err != nil {
		resp.Diagnostics.AddError("resolve content", err.Error())
		return
	}
	if err := r.client.PutResolvedConf(ctx, body); err != nil {
		resp.Diagnostics.AddError("create resolved.conf", err.Error())
		return
	}
	plan.Content = types.StringValue(body)
	plan.ID = types.StringValue("resolved.conf")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *resolvedResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resolvedModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	content, err := r.client.GetResolvedConf(ctx)
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.Content = types.StringValue(content)
	state.ID = types.StringValue("resolved.conf")
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *resolvedResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan resolvedModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, err := resolveFileContent(plan.Content, plan.Sections)
	if err != nil {
		resp.Diagnostics.AddError("resolve content", err.Error())
		return
	}
	if err := r.client.PutResolvedConf(ctx, body); err != nil {
		resp.Diagnostics.AddError("update resolved.conf", err.Error())
		return
	}
	plan.Content = types.StringValue(body)
	plan.ID = types.StringValue("resolved.conf")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *resolvedResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resolvedModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteResolvedConf(ctx); err != nil {
		resp.Diagnostics.AddError("delete resolved.conf", err.Error())
	}
}

func (r *resolvedResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
