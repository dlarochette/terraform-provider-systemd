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

var _ resource.Resource = &machineResource{}
var _ resource.ResourceWithImportState = &machineResource{}
var _ resource.ResourceWithValidateConfig = &machineResource{}

func NewMachineResource() resource.Resource { return &machineResource{} }

type machineResource struct {
	client *Client
}

type machineImageModel struct {
	Type   types.String `tfsdk:"type"`
	Source types.String `tfsdk:"source"`
}

type machineModel struct {
	Name        types.String       `tfsdk:"name"`
	Image       machineImageModel  `tfsdk:"image"`
	Content     types.String       `tfsdk:"content"`
	Sections    []sectionModel     `tfsdk:"section"`
	Enable      types.Bool         `tfsdk:"enable"`
	Active      types.Bool         `tfsdk:"active"`
	DeleteImage types.Bool         `tfsdk:"delete_image"`
	ID          types.String       `tfsdk:"id"`
}

func (r *machineResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_machine"
}

func (r *machineResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a systemd-nspawn machine: image under `/var/lib/machines/{name}`, " +
			"optional settings in `/etc/systemd/nspawn/{name}.nspawn`, and lifecycle of " +
			"`systemd-nspawn@{name}.service` via `systemctl`.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Machine name (also the image name). Forces replacement when changed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					noPathSegment(),
				},
			},
			"content": optionalContentAttribute(),
			"enable": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether `systemd-nspawn@{name}.service` should be enabled.",
			},
			"active": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether the machine should be started (`systemctl start` / `stop`).",
			},
			"delete_image": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "On destroy, run `machinectl remove` for the image (default `true`).",
				Default:             booldefault.StaticBool(true),
			},
			"id": idAttribute(),
		},
		Blocks: map[string]schema.Block{
			"image": schema.SingleNestedBlock{
				MarkdownDescription: "How to obtain the machine image (required).",
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Image source kind: `local` (import-tar/import-raw), `tar` (pull-tar), `raw` (pull-raw), or `oci` (pull-dkr).",
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
						Validators: []validator.String{
							stringvalidator.OneOf("local", "tar", "raw", "oci"),
						},
					},
					"source": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Remote path (local), HTTPS URL (tar/raw), or OCI reference (oci).",
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
				},
			},
			"section": sectionBlockSchema(),
		},
	}
}

func (r *machineResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg machineModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.Image.Type.IsNull() || cfg.Image.Source.IsNull() ||
		cfg.Image.Type.IsUnknown() && cfg.Image.Source.IsUnknown() {
		// When the whole block is omitted both are null.
		if cfg.Image.Type.IsNull() || cfg.Image.Source.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("image"), "Missing image", "An image block with type and source is required.")
			return
		}
	}
	hasC := !cfg.Content.IsNull() && !cfg.Content.IsUnknown() && cfg.Content.ValueString() != ""
	hasS := len(cfg.Sections) > 0
	if hasC && hasS {
		resp.Diagnostics.AddError("Invalid configuration", "content and section are mutually exclusive; set only one")
	}
}

func (r *machineResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *machineResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan machineModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan, true); err != nil {
		resp.Diagnostics.AddError("create machine", err.Error())
		return
	}
	plan.ID = plan.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *machineResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state machineModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	st, err := r.client.GetMachine(ctx, state.Name.ValueString())
	if err != nil || !st.ImagePresent {
		resp.State.RemoveResource(ctx)
		return
	}
	if st.Settings != "" {
		state.Content = types.StringValue(st.Settings)
	}
	state.ID = state.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *machineResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan machineModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan, false); err != nil {
		resp.Diagnostics.AddError("update machine", err.Error())
		return
	}
	plan.ID = plan.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *machineResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state machineModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteImage := true
	if !state.DeleteImage.IsNull() && !state.DeleteImage.IsUnknown() {
		deleteImage = state.DeleteImage.ValueBool()
	}
	if err := r.client.DeleteMachine(ctx, state.Name.ValueString(), deleteImage); err != nil {
		resp.Diagnostics.AddError("delete machine", err.Error())
	}
}

func (r *machineResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func (r *machineResource) apply(ctx context.Context, plan *machineModel, create bool) error {
	if plan.Image.Type.IsNull() || plan.Image.Source.IsNull() {
		return fmt.Errorf("image block is required")
	}
	body, hasSettings, err := resolveOptionalMachineSettings(plan.Content, plan.Sections)
	if err != nil {
		return err
	}
	var settings *string
	switch {
	case hasSettings:
		settings = &body
		plan.Content = types.StringValue(body)
	case create:
		settings = nil
	default:
		// Update with neither content nor sections → clear .nspawn
		empty := ""
		settings = &empty
		plan.Content = types.StringNull()
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
	return r.client.PutMachine(ctx, plan.Name.ValueString(), plan.Image.Type.ValueString(), plan.Image.Source.ValueString(), settings, enable, active)
}

// resolveOptionalMachineSettings allows neither content nor sections (no .nspawn file).
func resolveOptionalMachineSettings(content types.String, sections []sectionModel) (body string, has bool, err error) {
	hasC := !content.IsNull() && !content.IsUnknown() && content.ValueString() != ""
	hasS := len(sections) > 0
	if hasC && hasS {
		return "", false, fmt.Errorf("content and section are mutually exclusive; set only one")
	}
	if !hasC && !hasS {
		return "", false, nil
	}
	body, err = resolveFileContent(content, sections)
	if err != nil {
		return "", false, err
	}
	return body, true, nil
}
