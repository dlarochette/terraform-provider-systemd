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
)

var _ resource.Resource = &dropinResource{}
var _ resource.ResourceWithImportState = &dropinResource{}

func NewDropinResource() resource.Resource { return &dropinResource{} }

type dropinResource struct{ client *Client }

type dropinModel struct {
	Unit    types.String `tfsdk:"unit"`
	Dropin  types.String `tfsdk:"dropin"`
	Content types.String `tfsdk:"content"`
	ID      types.String `tfsdk:"id"`
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
			"content": contentAttribute(),
			"id":      idAttribute(),
		},
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
	var plan dropinModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.PutDropin(ctx, plan.Unit.ValueString(), plan.Dropin.ValueString(), plan.Content.ValueString()); err != nil {
		resp.Diagnostics.AddError("create dropin", err.Error())
		return
	}
	plan.ID = types.StringValue(plan.Unit.ValueString() + "/" + plan.Dropin.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dropinResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dropinModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	content, err := r.client.GetDropin(ctx, state.Unit.ValueString(), state.Dropin.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.Content = types.StringValue(content)
	state.ID = types.StringValue(state.Unit.ValueString() + "/" + state.Dropin.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *dropinResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan dropinModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.PutDropin(ctx, plan.Unit.ValueString(), plan.Dropin.ValueString(), plan.Content.ValueString()); err != nil {
		resp.Diagnostics.AddError("update dropin", err.Error())
		return
	}
	plan.ID = types.StringValue(plan.Unit.ValueString() + "/" + plan.Dropin.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dropinResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dropinModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteDropin(ctx, state.Unit.ValueString(), state.Dropin.ValueString()); err != nil {
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
