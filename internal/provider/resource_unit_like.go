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

// unitLikeResource manages a typed systemd unit file (.timer, .mount, .path, .swap, .slice, …)
// under /etc/systemd/system via the same SSH + systemctl path as systemd_unit.
type unitLikeResource struct {
	client   *Client
	typeName string
	suffix   string
	doc      string
	sections []string
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
		sections: sdprops.SectionsForUnitType("Timer"),
		typeName: "_timer",
		suffix:   ".timer",
		doc:      "Manages a systemd `.timer` unit under `/etc/systemd/system`. Pair with a matching `.service` (`systemd_unit`).",
	}
}

func NewMountResource() resource.Resource {
	return &unitLikeResource{
		sections: sdprops.SectionsForUnitType("Mount"),
		typeName: "_mount",
		suffix:   ".mount",
		doc:      "Manages a systemd `.mount` unit under `/etc/systemd/system` (e.g. `data.mount` for `/data`).",
	}
}

func NewAutomountResource() resource.Resource {
	return &unitLikeResource{
		sections: sdprops.SectionsForUnitType("Automount"),
		typeName: "_automount",
		suffix:   ".automount",
		doc:      "Manages a systemd `.automount` unit under `/etc/systemd/system`. Usually paired with a `.mount` unit.",
	}
}

func NewSocketResource() resource.Resource {
	return &unitLikeResource{
		sections: sdprops.SectionsForUnitType("Socket"),
		typeName: "_socket",
		suffix:   ".socket",
		doc:      "Manages a systemd `.socket` unit under `/etc/systemd/system`. Pair with a matching `.service`.",
	}
}

func NewPathResource() resource.Resource {
	return &unitLikeResource{
		sections: sdprops.SectionsForUnitType("Path"),
		typeName: "_path",
		suffix:   ".path",
		doc:      "Manages a systemd `.path` unit under `/etc/systemd/system`. Pair with a matching `.service` (`PathExists` / `PathChanged` / …).",
	}
}

func NewSwapResource() resource.Resource {
	return &unitLikeResource{
		sections: sdprops.SectionsForUnitType("Swap"),
		typeName: "_swap",
		suffix:   ".swap",
		doc:      "Manages a systemd `.swap` unit under `/etc/systemd/system` (e.g. `swapfile.swap`).",
	}
}

func NewSliceResource() resource.Resource {
	return &unitLikeResource{
		sections: sdprops.SectionsForUnitType("Slice"),
		typeName: "_slice",
		suffix:   ".slice",
		doc:      "Manages a systemd `.slice` unit under `/etc/systemd/system` (cgroup resource hierarchy).",
	}
}

func NewTargetResource() resource.Resource {
	return &unitLikeResource{
		sections: sdprops.SectionsForUnitType("Target"),
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
	for name, block := range typedBlocks(r.sections) {
		resp.Schema.Blocks[name] = block
	}
}

func (r *unitLikeResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	d, err := unitLikeFromRaw(req.Config.Raw)
	if err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
		return
	}
	content := types.StringNull()
	if d.HasContent {
		content = types.StringValue(d.Content)
	}
	if err := validateContentSectionsTyped(content, d.Sections, req.Config.Raw, r.sections); err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
		return
	}
	if err := validateTypedConflicts(req.Config.Raw, d.Sections, r.sections); err != nil {
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
	d, err := unitLikeFromRaw(req.Plan.Raw)
	if err != nil {
		resp.Diagnostics.AddError("read plan", err.Error())
		return
	}
	content := types.StringNull()
	if d.HasContent {
		content = types.StringValue(d.Content)
	}
	body, err := resolveFileContentTyped(content, d.Sections, req.Plan.Raw, r.sections)
	if err != nil {
		resp.Diagnostics.AddError("resolve content", err.Error())
		return
	}
	v, verr := r.client.hostSystemdVersion()
	if verr != nil {
		resp.Diagnostics.AddWarning("systemd version", "cannot determine the remote systemd release ("+verr.Error()+"); directive availability was not checked")
	} else if err := validateTypedForVersion(req.Plan.Raw, r.sections, v); err != nil {
		resp.Diagnostics.AddError("Incompatible with the target systemd version", err.Error())
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

func (r *unitLikeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
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

func (r *unitLikeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	d, err := unitLikeFromRaw(req.Plan.Raw)
	if err != nil {
		resp.Diagnostics.AddError("read plan", err.Error())
		return
	}
	content := types.StringNull()
	if d.HasContent {
		content = types.StringValue(d.Content)
	}
	body, err := resolveFileContentTyped(content, d.Sections, req.Plan.Raw, r.sections)
	if err != nil {
		resp.Diagnostics.AddError("resolve content", err.Error())
		return
	}
	v, verr := r.client.hostSystemdVersion()
	if verr != nil {
		resp.Diagnostics.AddWarning("systemd version", "cannot determine the remote systemd release ("+verr.Error()+"); directive availability was not checked")
	} else if err := validateTypedForVersion(req.Plan.Raw, r.sections, v); err != nil {
		resp.Diagnostics.AddError("Incompatible with the target systemd version", err.Error())
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

func (r *unitLikeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	d, err := unitLikeFromRaw(req.State.Raw)
	if err != nil {
		resp.Diagnostics.AddError("read state", err.Error())
		return
	}
	if err := r.client.DeleteUnit(ctx, d.Name); err != nil {
		resp.Diagnostics.AddError("delete unit", err.Error())
	}
}

func (r *unitLikeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
