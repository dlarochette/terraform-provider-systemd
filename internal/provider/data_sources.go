package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &unitDataSource{}

func NewUnitDataSource() datasource.DataSource { return &unitDataSource{} }

type unitDataSource struct{ client *Client }

type unitDataModel struct {
	Name          types.String `tfsdk:"name"`
	LoadState     types.String `tfsdk:"load_state"`
	ActiveState   types.String `tfsdk:"active_state"`
	SubState      types.String `tfsdk:"sub_state"`
	UnitFileState types.String `tfsdk:"unit_file_state"`
	ID            types.String `tfsdk:"id"`
}

func (d *unitDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_unit"
}

func (d *unitDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads systemd unit status via remote `systemctl show`.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Unit filename (e.g. `sshd.service`).",
				Validators: []validator.String{
					unitNameValidator(),
				},
			},
			"load_state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "systemd `LoadState` property.",
			},
			"active_state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "systemd `ActiveState` property.",
			},
			"sub_state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "systemd `SubState` property.",
			},
			"unit_file_state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "systemd `UnitFileState` property (enabled/disabled/…).",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Same as `name`.",
			},
		},
	}
}

func (d *unitDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, err := clientFrom(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("configure", err.Error())
		return
	}
	d.client = c
}

func (d *unitDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg unitDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	st, err := d.client.UnitStatus(ctx, cfg.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("unit status", err.Error())
		return
	}
	cfg.LoadState = types.StringValue(st.LoadState)
	cfg.ActiveState = types.StringValue(st.ActiveState)
	cfg.SubState = types.StringValue(st.SubState)
	cfg.UnitFileState = types.StringValue(st.UnitFileState)
	cfg.ID = cfg.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

var _ datasource.DataSource = &linkDataSource{}

func NewLinkDataSource() datasource.DataSource { return &linkDataSource{} }

type linkDataSource struct{ client *Client }

type linkDataModel struct {
	Name             types.String `tfsdk:"name"`
	OperationalState types.String `tfsdk:"operational_state"`
	SetupState       types.String `tfsdk:"setup_state"`
	ID               types.String `tfsdk:"id"`
}

func (d *linkDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_link"
}

func (d *linkDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads networkd link status via remote `networkctl status`.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Interface name (e.g. `eth0`).",
				Validators: []validator.String{
					noPathSegment(),
				},
			},
			"operational_state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Parsed operational state from `networkctl status`.",
			},
			"setup_state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Parsed setup state from `networkctl status` when present.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Same as `name`.",
			},
		},
	}
}

func (d *linkDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, err := clientFrom(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("configure", err.Error())
		return
	}
	d.client = c
}

func (d *linkDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg linkDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	st, err := d.client.LinkStatus(ctx, cfg.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("link status", err.Error())
		return
	}
	cfg.OperationalState = types.StringValue(st.OperationalState)
	cfg.SetupState = types.StringValue(st.SetupState)
	cfg.ID = cfg.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

var _ datasource.DataSource = &resolveStatusDataSource{}

func NewResolveStatusDataSource() datasource.DataSource { return &resolveStatusDataSource{} }

type resolveStatusDataSource struct{ client *Client }

type resolveStatusModel struct {
	Link   types.String `tfsdk:"link"`
	Status types.String `tfsdk:"status"`
	ID     types.String `tfsdk:"id"`
}

func (d *resolveStatusDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resolve_status"
}

func (d *resolveStatusDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads systemd-resolved status via remote `resolvectl status [link]`.",
		Attributes: map[string]schema.Attribute{
			"link": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional interface name. When unset, returns global status.",
				Validators: []validator.String{
					noPathSegment(),
				},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Raw stdout from `resolvectl status`.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Link name, or `global` when unset.",
			},
		},
	}
}

func (d *resolveStatusDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, err := clientFrom(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("configure", err.Error())
		return
	}
	d.client = c
}

func (d *resolveStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg resolveStatusModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	link := ""
	if !cfg.Link.IsNull() && !cfg.Link.IsUnknown() {
		link = cfg.Link.ValueString()
	}
	out, err := d.client.ResolveStatus(ctx, link)
	if err != nil {
		resp.Diagnostics.AddError("resolve status", err.Error())
		return
	}
	cfg.Status = types.StringValue(out)
	if link == "" {
		cfg.ID = types.StringValue("global")
	} else {
		cfg.ID = types.StringValue(link)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
