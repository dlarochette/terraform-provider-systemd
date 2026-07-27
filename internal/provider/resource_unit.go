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

var _ resource.Resource = &unitResource{}
var _ resource.ResourceWithImportState = &unitResource{}

func NewUnitResource() resource.Resource { return &unitResource{} }

type unitResource struct {
	client *Client
}

type unitModel struct {
	Name    types.String `tfsdk:"name"`
	Content types.String `tfsdk:"content"`
	Enable  types.Bool   `tfsdk:"enable"`
	Active  types.Bool   `tfsdk:"active"`
	ID      types.String `tfsdk:"id"`
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
			"content": contentAttribute(),
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
	var plan unitModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var enable, active *bool
	if !plan.Enable.IsNull() {
		v := plan.Enable.ValueBool()
		enable = &v
	}
	if !plan.Active.IsNull() {
		v := plan.Active.ValueBool()
		active = &v
	}
	if err := r.client.PutUnit(ctx, plan.Name.ValueString(), plan.Content.ValueString(), enable, active); err != nil {
		resp.Diagnostics.AddError("create unit", err.Error())
		return
	}
	plan.ID = plan.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *unitResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state unitModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	content, err := r.client.GetUnit(ctx, state.Name.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.Content = types.StringValue(content)
	state.ID = state.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *unitResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan unitModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var enable, active *bool
	if !plan.Enable.IsNull() {
		v := plan.Enable.ValueBool()
		enable = &v
	}
	if !plan.Active.IsNull() {
		v := plan.Active.ValueBool()
		active = &v
	}
	if err := r.client.PutUnit(ctx, plan.Name.ValueString(), plan.Content.ValueString(), enable, active); err != nil {
		resp.Diagnostics.AddError("update unit", err.Error())
		return
	}
	plan.ID = plan.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *unitResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state unitModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteUnit(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("delete unit", err.Error())
	}
}

func (r *unitResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
