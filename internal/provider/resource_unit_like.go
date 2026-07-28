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

// unitLikeResource manages a typed systemd unit file (.timer, .mount, .path, .swap, .slice, …)
// under /etc/systemd/system via the same SSH + systemctl path as systemd_unit.
type unitLikeResource struct {
	client   *Client
	typeName string
	suffix   string
	doc      string
}

type unitLikeModel struct {
	Name     types.String   `tfsdk:"name"`
	Content  types.String   `tfsdk:"content"`
	Sections []sectionModel `tfsdk:"section"`
	Enable   types.Bool     `tfsdk:"enable"`
	Active   types.Bool     `tfsdk:"active"`
	ID       types.String   `tfsdk:"id"`
}

func NewTimerResource() resource.Resource {
	return &unitLikeResource{
		typeName: "_timer",
		suffix:   ".timer",
		doc:      "Manages a systemd `.timer` unit under `/etc/systemd/system`. Pair with a matching `.service` (`systemd_unit`).",
	}
}

func NewMountResource() resource.Resource {
	return &unitLikeResource{
		typeName: "_mount",
		suffix:   ".mount",
		doc:      "Manages a systemd `.mount` unit under `/etc/systemd/system` (e.g. `data.mount` for `/data`).",
	}
}

func NewAutomountResource() resource.Resource {
	return &unitLikeResource{
		typeName: "_automount",
		suffix:   ".automount",
		doc:      "Manages a systemd `.automount` unit under `/etc/systemd/system`. Usually paired with a `.mount` unit.",
	}
}

func NewSocketResource() resource.Resource {
	return &unitLikeResource{
		typeName: "_socket",
		suffix:   ".socket",
		doc:      "Manages a systemd `.socket` unit under `/etc/systemd/system`. Pair with a matching `.service`.",
	}
}

func NewPathResource() resource.Resource {
	return &unitLikeResource{
		typeName: "_path",
		suffix:   ".path",
		doc:      "Manages a systemd `.path` unit under `/etc/systemd/system`. Pair with a matching `.service` (`PathExists` / `PathChanged` / …).",
	}
}

func NewSwapResource() resource.Resource {
	return &unitLikeResource{
		typeName: "_swap",
		suffix:   ".swap",
		doc:      "Manages a systemd `.swap` unit under `/etc/systemd/system` (e.g. `swapfile.swap`).",
	}
}

func NewSliceResource() resource.Resource {
	return &unitLikeResource{
		typeName: "_slice",
		suffix:   ".slice",
		doc:      "Manages a systemd `.slice` unit under `/etc/systemd/system` (cgroup resource hierarchy).",
	}
}

func NewTargetResource() resource.Resource {
	return &unitLikeResource{
		typeName: "_target",
		suffix:   ".target",
		doc:      "Manages a systemd `.target` unit under `/etc/systemd/system` (grouping / synchronization point for other units).",
	}
}

var _ resource.Resource = &unitLikeResource{}
var _ resource.ResourceWithImportState = &unitLikeResource{}
var _ resource.ResourceWithValidateConfig = &unitLikeResource{}

func (r *unitLikeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + r.typeName
}

func (r *unitLikeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: r.doc,
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Unit filename ending with `" + r.suffix + "`. Forces replacement when changed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					unitSuffixValidator(r.suffix),
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
}

func (r *unitLikeResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg unitLikeModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := validateContentOrSections(cfg.Content, cfg.Sections); err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
	}
}

func (r *unitLikeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *unitLikeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan unitLikeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, err := resolveFileContent(plan.Content, plan.Sections)
	if err != nil {
		resp.Diagnostics.AddError("resolve content", err.Error())
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
	if err := r.client.PutUnit(ctx, plan.Name.ValueString(), body, enable, active); err != nil {
		resp.Diagnostics.AddError("create unit", err.Error())
		return
	}
	plan.Content = types.StringValue(body)
	plan.ID = plan.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *unitLikeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state unitLikeModel
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

func (r *unitLikeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan unitLikeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, err := resolveFileContent(plan.Content, plan.Sections)
	if err != nil {
		resp.Diagnostics.AddError("resolve content", err.Error())
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
	if err := r.client.PutUnit(ctx, plan.Name.ValueString(), body, enable, active); err != nil {
		resp.Diagnostics.AddError("update unit", err.Error())
		return
	}
	plan.Content = types.StringValue(body)
	plan.ID = plan.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *unitLikeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state unitLikeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteUnit(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("delete unit", err.Error())
	}
}

func (r *unitLikeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
