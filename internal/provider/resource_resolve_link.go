package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &resolveLinkResource{}
var _ resource.ResourceWithImportState = &resolveLinkResource{}

func NewResolveLinkResource() resource.Resource { return &resolveLinkResource{} }

type resolveLinkResource struct{ client *Client }

type resolveLinkModel struct {
	Link         types.String `tfsdk:"link"`
	DNS          types.List   `tfsdk:"dns"`
	Domains      types.List   `tfsdk:"domains"`
	DefaultRoute types.Bool   `tfsdk:"default_route"`
	LLMNR        types.String `tfsdk:"llmnr"`
	MDNS         types.String `tfsdk:"mdns"`
	DNSSEC       types.String `tfsdk:"dnssec"`
	DNSOverTLS   types.String `tfsdk:"dnsovertls"`
	ID           types.String `tfsdk:"id"`
}

func (r *resolveLinkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resolve_link"
}

func (r *resolveLinkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages per-link DNS settings via remote `resolvectl`. " +
			"**Runtime-only**: settings are cleared on reboot unless also configured in a `.network` file. " +
			"Destroy runs `resolvectl revert <link>`.",
		Attributes: map[string]schema.Attribute{
			"link": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Network interface name (e.g. `eth0`). Forces replacement when changed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					noPathSegment(),
				},
			},
			"dns": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "DNS servers for the link (`resolvectl dns`).",
			},
			"domains": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Search/routing domains (`resolvectl domain`). Use `~` prefix for route-only domains.",
			},
			"default_route": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether the link is used as a default route for DNS queries.",
			},
			"llmnr": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "LLMNR mode as accepted by `resolvectl llmnr` (e.g. `yes`, `no`, `resolve`).",
			},
			"mdns": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "mDNS mode as accepted by `resolvectl mdns`.",
			},
			"dnssec": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "DNSSEC mode as accepted by `resolvectl dnssec`.",
			},
			"dnsovertls": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "DNS-over-TLS mode as accepted by `resolvectl dnsovertls`.",
			},
			"id": idAttribute(),
		},
	}
}

func (r *resolveLinkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func desiredFromModel(ctx context.Context, m resolveLinkModel) (ResolveLinkDesired, error) {
	d := ResolveLinkDesired{}
	if !m.DNS.IsNull() && !m.DNS.IsUnknown() {
		var dns []string
		diags := m.DNS.ElementsAs(ctx, &dns, false)
		if diags.HasError() {
			return d, fmt.Errorf("%s", diags.Errors()[0].Detail())
		}
		d.DNS = dns
	}
	if !m.Domains.IsNull() && !m.Domains.IsUnknown() {
		var domains []string
		diags := m.Domains.ElementsAs(ctx, &domains, false)
		if diags.HasError() {
			return d, fmt.Errorf("%s", diags.Errors()[0].Detail())
		}
		d.Domains = domains
	}
	if !m.DefaultRoute.IsNull() && !m.DefaultRoute.IsUnknown() {
		v := m.DefaultRoute.ValueBool()
		d.DefaultRoute = &v
	}
	if !m.LLMNR.IsNull() && !m.LLMNR.IsUnknown() {
		d.LLMNR = m.LLMNR.ValueString()
	}
	if !m.MDNS.IsNull() && !m.MDNS.IsUnknown() {
		d.MDNS = m.MDNS.ValueString()
	}
	if !m.DNSSEC.IsNull() && !m.DNSSEC.IsUnknown() {
		d.DNSSEC = m.DNSSEC.ValueString()
	}
	if !m.DNSOverTLS.IsNull() && !m.DNSOverTLS.IsUnknown() {
		d.DNSOverTLS = m.DNSOverTLS.ValueString()
	}
	return d, nil
}

func stringListValue(vals []string) types.List {
	elems := make([]attr.Value, len(vals))
	for i, v := range vals {
		elems[i] = types.StringValue(v)
	}
	return types.ListValueMust(types.StringType, elems)
}

func (r *resolveLinkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan resolveLinkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	desired, err := desiredFromModel(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("resolve link config", err.Error())
		return
	}
	if err := r.client.PutResolveLink(ctx, plan.Link.ValueString(), desired); err != nil {
		resp.Diagnostics.AddError("create resolve link", err.Error())
		return
	}
	plan.ID = plan.Link
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *resolveLinkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state resolveLinkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	link := state.Link.ValueString()
	if !state.DNS.IsNull() {
		dns, err := r.client.GetResolveLinkDNS(ctx, link)
		if err != nil {
			resp.Diagnostics.AddError("read resolve link dns", err.Error())
			return
		}
		state.DNS = stringListValue(dns)
	}
	if !state.Domains.IsNull() {
		domains, err := r.client.GetResolveLinkDomains(ctx, link)
		if err != nil {
			resp.Diagnostics.AddError("read resolve link domains", err.Error())
			return
		}
		state.Domains = stringListValue(domains)
	}
	state.ID = state.Link
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *resolveLinkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan resolveLinkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	desired, err := desiredFromModel(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("resolve link config", err.Error())
		return
	}
	if err := r.client.PutResolveLink(ctx, plan.Link.ValueString(), desired); err != nil {
		resp.Diagnostics.AddError("update resolve link", err.Error())
		return
	}
	plan.ID = plan.Link
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *resolveLinkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state resolveLinkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteResolveLink(ctx, state.Link.ValueString()); err != nil {
		resp.Diagnostics.AddError("delete resolve link", err.Error())
	}
}

func (r *resolveLinkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("link"), req, resp)
}
