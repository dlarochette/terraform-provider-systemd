package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerSchema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"fmt"

	"github.com/dlarochette/terraform-provider-systemd/internal/remote"
)

var _ provider.Provider = &SystemdProvider{}

// SystemdProvider implements the systemd provider.
type SystemdProvider struct {
	version string
}

type providerModel struct {
	Host                  types.String `tfsdk:"host"`
	User                  types.String `tfsdk:"user"`
	Port                  types.Int64  `tfsdk:"port"`
	PrivateKey            types.String `tfsdk:"private_key"`
	PrivateKeyPath        types.String `tfsdk:"private_key_path"`
	SSHAgent              types.Bool   `tfsdk:"ssh_agent"`
	BastionHost           types.String `tfsdk:"bastion_host"`
	BastionUser           types.String `tfsdk:"bastion_user"`
	BastionPort           types.Int64  `tfsdk:"bastion_port"`
	InsecureIgnoreHostKey types.Bool   `tfsdk:"insecure_ignore_host_key"`
}

type providerData struct {
	client *Client
}

// New returns a provider factory.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &SystemdProvider{version: version}
	}
}

func (p *SystemdProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "systemd"
	resp.Version = p.version
}

func (p *SystemdProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = providerSchema.Schema{
		MarkdownDescription: "Manage systemd units and networkd files on remote hosts over SSH (SFTP + systemctl/networkctl).",
		Attributes: map[string]providerSchema.Attribute{
			"host": providerSchema.StringAttribute{
				Required:            true,
				MarkdownDescription: "SSH hostname or IP address of the target machine.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"user": providerSchema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "SSH user (default `root`).",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"port": providerSchema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "SSH port (default `22`).",
				Validators: []validator.Int64{
					portValidator(),
				},
			},
			"private_key": providerSchema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "PEM-encoded private key contents. Conflicts with `private_key_path`.",
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("private_key_path")),
				},
			},
			"private_key_path": providerSchema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Path to a PEM private key file. Conflicts with `private_key`.",
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("private_key")),
					stringvalidator.LengthAtLeast(1),
				},
			},
			"ssh_agent": providerSchema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Use the local SSH agent (`SSH_AUTH_SOCK`). Defaults to true when no key is set.",
			},
			"bastion_host": providerSchema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional SSH jump host.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"bastion_user": providerSchema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "SSH user on the bastion (defaults to `user`).",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"bastion_port": providerSchema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Bastion SSH port (default `22`).",
				Validators: []validator.Int64{
					portValidator(),
				},
			},
			"insecure_ignore_host_key": providerSchema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Skip `known_hosts` verification (lab only).",
			},
		},
	}
}

func (p *SystemdProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if cfg.Host.IsNull() || cfg.Host.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(path.Root("host"), "Missing host", "host is required")
		return
	}

	sc := remote.Config{
		Host:                  cfg.Host.ValueString(),
		User:                  cfg.User.ValueString(),
		PrivateKey:            cfg.PrivateKey.ValueString(),
		PrivateKeyPath:        cfg.PrivateKeyPath.ValueString(),
		BastionHost:           cfg.BastionHost.ValueString(),
		BastionUser:           cfg.BastionUser.ValueString(),
		InsecureIgnoreHostKey: !cfg.InsecureIgnoreHostKey.IsNull() && cfg.InsecureIgnoreHostKey.ValueBool(),
		UseSSHAgent:           cfg.SSHAgent.IsNull() || cfg.SSHAgent.ValueBool(),
	}
	if !cfg.Port.IsNull() {
		sc.Port = int(cfg.Port.ValueInt64())
	}
	if !cfg.BastionPort.IsNull() {
		sc.BastionPort = int(cfg.BastionPort.ValueInt64())
	}

	host, err := remote.Dial(sc)
	if err != nil {
		resp.Diagnostics.AddError("SSH dial failed", err.Error())
		return
	}

	data := &providerData{client: &Client{Host: host}}
	tflog.Info(ctx, "configured systemd provider over SSH", map[string]any{"host": sc.Host})
	resp.ResourceData = data
	resp.DataSourceData = data
}

func (p *SystemdProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewUnitResource,
		NewTimerResource,
		NewMountResource,
		NewAutomountResource,
		NewSocketResource,
		NewPathResource,
		NewSwapResource,
		NewSliceResource,
		NewTargetResource,
		NewDropinResource,
		NewNetworkResource,
		NewNetdevResource,
		NewLinkResource,
		NewInstanceResource,
		NewCredentialResource,
		NewMachineResource,
	}
}

func (p *SystemdProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewUnitDataSource,
		NewLinkDataSource,
	}
}

func clientFrom(meta any) (*Client, error) {
	d, ok := meta.(*providerData)
	if !ok || d == nil || d.client == nil {
		return nil, fmt.Errorf("provider not configured")
	}
	return d.client, nil
}

// Shared resource attribute helpers used by unit/dropin/network schemas.

func idAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "Resource identifier used in Terraform state.",
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
}
