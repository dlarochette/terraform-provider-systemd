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

var _ resource.Resource = &networkFileResource{}
var _ resource.ResourceWithImportState = &networkFileResource{}
var _ resource.ResourceWithValidateConfig = &networkFileResource{}

type networkFileResource struct {
	client   *Client
	typeName string
	doc      string
	suffix   string
}

type networkFileModel struct {
	Filename types.String   `tfsdk:"filename"`
	Content  types.String   `tfsdk:"content"`
	Sections []sectionModel `tfsdk:"section"`
	ID       types.String   `tfsdk:"id"`
}

func NewNetworkResource() resource.Resource {
	return &networkFileResource{
		typeName: "_network",
		doc:      "Manages a `.network` file under `/etc/systemd/network` over SSH.",
		suffix:   ".network",
	}
}
func NewNetdevResource() resource.Resource {
	return &networkFileResource{
		typeName: "_netdev",
		doc:      "Manages a `.netdev` file under `/etc/systemd/network` over SSH.",
		suffix:   ".netdev",
	}
}
func NewLinkResource() resource.Resource {
	return &networkFileResource{
		typeName: "_link",
		doc:      "Manages a `.link` file under `/etc/systemd/network` over SSH.",
		suffix:   ".link",
	}
}

func (r *networkFileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + r.typeName
}

func (r *networkFileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: r.doc,
		Attributes: map[string]schema.Attribute{
			"filename": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Filename under `/etc/systemd/network` (must end with `" + r.suffix + "`). Forces replacement when changed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					networkFilenameValidator(r.suffix),
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

func (r *networkFileResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg networkFileModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := validateContentOrSections(cfg.Content, cfg.Sections); err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
	}
}

func (r *networkFileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *networkFileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan networkFileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, err := resolveFileContent(plan.Content, plan.Sections)
	if err != nil {
		resp.Diagnostics.AddError("resolve content", err.Error())
		return
	}
	if err := r.client.PutNetwork(ctx, plan.Filename.ValueString(), body); err != nil {
		resp.Diagnostics.AddError("create network file", err.Error())
		return
	}
	plan.Content = types.StringValue(body)
	plan.ID = plan.Filename
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *networkFileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state networkFileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	content, err := r.client.GetNetwork(ctx, state.Filename.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.Content = types.StringValue(content)
	state.ID = state.Filename
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *networkFileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan networkFileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body, err := resolveFileContent(plan.Content, plan.Sections)
	if err != nil {
		resp.Diagnostics.AddError("resolve content", err.Error())
		return
	}
	if err := r.client.PutNetwork(ctx, plan.Filename.ValueString(), body); err != nil {
		resp.Diagnostics.AddError("update network file", err.Error())
		return
	}
	plan.Content = types.StringValue(body)
	plan.ID = plan.Filename
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *networkFileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state networkFileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteNetwork(ctx, state.Filename.ValueString()); err != nil {
		resp.Diagnostics.AddError("delete network file", err.Error())
	}
}

func (r *networkFileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("filename"), req, resp)
}
