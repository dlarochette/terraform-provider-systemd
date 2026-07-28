package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &portableResource{}
var _ resource.ResourceWithImportState = &portableResource{}
var _ resource.ResourceWithValidateConfig = &portableResource{}

func NewPortableResource() resource.Resource { return &portableResource{} }

type portableResource struct {
	client *Client
}

type portableModel struct {
	Name        types.String      `tfsdk:"name"`
	Image       machineImageModel `tfsdk:"image"`
	Enable      types.Bool        `tfsdk:"enable"`
	Active      types.Bool        `tfsdk:"active"`
	DeleteImage types.Bool        `tfsdk:"delete_image"`
	ID          types.String      `tfsdk:"id"`
}

func (r *portableResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_portable"
}

func (r *portableResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a systemd portable service image under `/var/lib/portables/{name}` " +
			"(or `{name}.raw`) and attaches it with `portablectl`. Unit overrides use `systemd_dropin` / unit resources. " +
			"`image.type = oci` is not supported (use `local`, `tar`, or `raw`).",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Portable image name (also the unit prefix, e.g. `app` → `app.service`). Forces replacement when changed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					noPathSegment(),
				},
			},
			"enable": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Pass `--enable` to `portablectl attach` / manage the primary `{name}.service` unit.",
			},
			"active": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Pass `--now` to `portablectl attach` / start the primary `{name}.service` unit.",
			},
			"delete_image": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "On destroy, remove the portable image from disk (default `true`).",
				Default:             booldefault.StaticBool(true),
			},
			"id": idAttribute(),
		},
		Blocks: map[string]schema.Block{
			"image": schema.SingleNestedBlock{
				MarkdownDescription: "How to obtain the portable image (required). `oci` is rejected.",
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "`local` (tar extract or raw copy), `tar` (HTTP pull+extract), or `raw` (HTTP pull to `.raw`).",
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
						Validators: []validator.String{
							stringvalidator.OneOf("local", "tar", "raw"),
						},
					},
					"source": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Remote path (local) or HTTPS URL (tar/raw).",
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
				},
			},
		},
	}
}

func (r *portableResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg portableModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.Image.Type.IsNull() || cfg.Image.Source.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("image"), "Missing image", "An image block with type and source is required.")
	}
}

func (r *portableResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *portableResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan portableModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("create portable", err.Error())
		return
	}
	plan.ID = plan.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *portableResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state portableModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	st, err := r.client.GetPortable(ctx, state.Name.ValueString())
	if err != nil || !st.ImagePresent {
		resp.State.RemoveResource(ctx)
		return
	}
	state.ID = state.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *portableResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan portableModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("update portable", err.Error())
		return
	}
	plan.ID = plan.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *portableResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state portableModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteImage := true
	if !state.DeleteImage.IsNull() && !state.DeleteImage.IsUnknown() {
		deleteImage = state.DeleteImage.ValueBool()
	}
	if err := r.client.DeletePortable(ctx, state.Name.ValueString(), deleteImage); err != nil {
		resp.Diagnostics.AddError("delete portable", err.Error())
	}
}

func (r *portableResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func (r *portableResource) apply(ctx context.Context, plan *portableModel) error {
	if plan.Image.Type.IsNull() || plan.Image.Source.IsNull() {
		return fmt.Errorf("image block is required")
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
	return r.client.PutPortable(ctx, plan.Name.ValueString(), plan.Image.Type.ValueString(), plan.Image.Source.ValueString(), enable, active)
}
