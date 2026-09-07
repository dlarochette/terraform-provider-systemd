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
	specs    []sectionSpec
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
		specs:    netSpecs("network"),
	}
}
func NewNetdevResource() resource.Resource {
	return &networkFileResource{
		typeName: "_netdev",
		doc:      "Manages a `.netdev` file under `/etc/systemd/network` over SSH.",
		suffix:   ".netdev",
		specs:    netSpecs("netdev"),
	}
}
func NewLinkResource() resource.Resource {
	return &networkFileResource{
		typeName: "_link",
		doc:      "Manages a `.link` file under `/etc/systemd/network` over SSH.",
		suffix:   ".link",
		specs:    netSpecs("link"),
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
	for name, block := range typedBlocks(r.specs) {
		resp.Schema.Blocks[name] = block
	}
}

func (r *networkFileResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	d, err := networkFileFromRaw(req.Config.Raw)
	if err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
		return
	}
	content := types.StringNull()
	if d.HasContent {
		content = types.StringValue(d.Content)
	}
	if err := validateContentSectionsTyped(content, d.Sections, req.Config.Raw, r.specs); err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
		return
	}
	if err := validateTypedConflicts(req.Config.Raw, d.Sections, r.specs); err != nil {
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
	d, err := networkFileFromRaw(req.Plan.Raw)
	if err != nil {
		resp.Diagnostics.AddError("read plan", err.Error())
		return
	}
	content := types.StringNull()
	if d.HasContent {
		content = types.StringValue(d.Content)
	}
	body, err := resolveFileContentTyped(content, d.Sections, req.Plan.Raw, r.specs)
	if err != nil {
		resp.Diagnostics.AddError("resolve content", err.Error())
		return
	}
	if err := r.client.PutNetwork(ctx, d.Filename, body); err != nil {
		resp.Diagnostics.AddError("create network file", err.Error())
		return
	}
	resp.State.Raw = setStateContentFilename(req.Plan.Raw, body, d.Filename)
}

func (r *networkFileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	d, err := networkFileFromRaw(req.State.Raw)
	if err != nil {
		resp.Diagnostics.AddError("read state", err.Error())
		return
	}
	content, err := r.client.GetNetwork(ctx, d.Filename)
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.State.Raw = setStateContentFilename(req.State.Raw, content, d.Filename)
}

func (r *networkFileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	d, err := networkFileFromRaw(req.Plan.Raw)
	if err != nil {
		resp.Diagnostics.AddError("read plan", err.Error())
		return
	}
	content := types.StringNull()
	if d.HasContent {
		content = types.StringValue(d.Content)
	}
	body, err := resolveFileContentTyped(content, d.Sections, req.Plan.Raw, r.specs)
	if err != nil {
		resp.Diagnostics.AddError("resolve content", err.Error())
		return
	}
	if err := r.client.PutNetwork(ctx, d.Filename, body); err != nil {
		resp.Diagnostics.AddError("update network file", err.Error())
		return
	}
	resp.State.Raw = setStateContentFilename(req.Plan.Raw, body, d.Filename)
}

func (r *networkFileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	d, err := networkFileFromRaw(req.State.Raw)
	if err != nil {
		resp.Diagnostics.AddError("read state", err.Error())
		return
	}
	if err := r.client.DeleteNetwork(ctx, d.Filename); err != nil {
		resp.Diagnostics.AddError("delete network file", err.Error())
	}
}

func (r *networkFileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("filename"), req, resp)
}
