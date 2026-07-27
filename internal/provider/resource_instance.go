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

// instanceResource manages the lifecycle (enable/active) of a systemd template
// unit instance (e.g. `app@bar.service` derived from template `app@.service`).
// It does not write a unit file; the template unit itself is managed by
// `systemd_unit` (or another typed unit resource).
type instanceResource struct {
	client *Client
}

type instanceModel struct {
	Template types.String `tfsdk:"template"`
	Instance types.String `tfsdk:"instance"`
	Enable   types.Bool   `tfsdk:"enable"`
	Active   types.Bool   `tfsdk:"active"`
	ID       types.String `tfsdk:"id"`
}

var _ resource.Resource = &instanceResource{}
var _ resource.ResourceWithImportState = &instanceResource{}

func NewInstanceResource() resource.Resource { return &instanceResource{} }

func (r *instanceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_instance"
}

func (r *instanceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the lifecycle (enable/active) of an instance of a systemd template unit " +
			"(e.g. `app@bar.service` instantiated from template `app@.service`). The template unit file itself " +
			"must be managed separately (e.g. with `systemd_unit`).",
		Attributes: map[string]schema.Attribute{
			"template": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Template unit name (e.g. `app@.service`). Forces replacement when changed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					templateNameValidator(),
				},
			},
			"instance": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Instance string spliced into the template (e.g. `bar` for `app@bar.service`). Forces replacement when changed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					instanceStringValidator(),
				},
			},
			"enable": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether the instance should be enabled (`systemctl enable` / `disable`).",
			},
			"active": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether the instance should be started (`systemctl start` / `stop`). Start failure fails the apply.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Instantiated unit name (e.g. `app@bar.service`).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *instanceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *instanceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan instanceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name, err := InstantiateUnit(plan.Template.ValueString(), plan.Instance.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("instantiate unit", err.Error())
		return
	}
	enable, active := boolPtr(plan.Enable), boolPtr(plan.Active)
	if err := r.client.ApplyUnitLifecycle(ctx, name, enable, active); err != nil {
		resp.Diagnostics.AddError("apply instance lifecycle", err.Error())
		return
	}
	plan.ID = types.StringValue(name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *instanceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state instanceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := state.ID.ValueString()
	st, err := r.client.UnitStatus(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("unit status", err.Error())
		return
	}
	if st.LoadState == "not-found" && st.UnitFileState == "" {
		resp.State.RemoveResource(ctx)
		return
	}
	// Desired enable/active state is config-driven; keep it as-is from state.
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *instanceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan instanceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name, err := InstantiateUnit(plan.Template.ValueString(), plan.Instance.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("instantiate unit", err.Error())
		return
	}
	enable, active := boolPtr(plan.Enable), boolPtr(plan.Active)
	if err := r.client.ApplyUnitLifecycle(ctx, name, enable, active); err != nil {
		resp.Diagnostics.AddError("apply instance lifecycle", err.Error())
		return
	}
	plan.ID = types.StringValue(name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *instanceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state instanceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client.DeleteUnitLifecycle(ctx, state.ID.ValueString())
}

func (r *instanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	template, instance, err := ParseInstantiatedUnit(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("import systemd_instance", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("template"), template)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("instance"), instance)...)
}

func boolPtr(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}
