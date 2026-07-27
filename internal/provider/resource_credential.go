package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &credentialResource{}
var _ resource.ResourceWithValidateConfig = &credentialResource{}

// credentialResource manages a systemd credential written to the host
// credential store (`/etc/credstore` or `/etc/credstore.encrypted`), meant to
// be consumed by units via `LoadCredential=`/`LoadCredentialEncrypted=`. The
// credential content is never read back from the remote host, so Terraform
// always keeps `data` as it appears in state/config — which means the
// secret is persisted in Terraform state (as a `Sensitive` attribute) and
// state must be treated/secured accordingly.
type credentialResource struct {
	client *Client
}

type credentialModel struct {
	Name      types.String `tfsdk:"name"`
	Data      types.String `tfsdk:"data"`
	Encrypted types.Bool   `tfsdk:"encrypted"`
	WithKey   types.String `tfsdk:"with_key"`
	ID        types.String `tfsdk:"id"`
}

func NewCredentialResource() resource.Resource { return &credentialResource{} }

func (r *credentialResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_credential"
}

func (r *credentialResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a systemd credential in the host credential store, consumable by units " +
			"via `LoadCredential=` (plaintext, `/etc/credstore/{name}`) or `LoadCredentialEncrypted=` " +
			"(encrypted with `systemd-creds`, `/etc/credstore.encrypted/{name}`). `data` is never decrypted or " +
			"read back from the remote host, so Terraform trusts state/config for drift — but that also means " +
			"the secret **is stored in Terraform state** (as a `Sensitive` attribute). Treat state as secret.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Credential filename under the credstore (e.g. `db-pass`). Forces replacement when changed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					noPathSegment(),
				},
			},
			"data": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
				MarkdownDescription: "Credential content. Never read back from the remote host, but persisted " +
					"in Terraform state (marked `Sensitive`) — treat state as secret.",
				Validators: []validator.String{
					nonEmptyContent(),
				},
			},
			"encrypted": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Encrypt the credential with `systemd-creds encrypt` under `/etc/credstore.encrypted` " +
					"(default `true`) instead of storing it as plaintext under `/etc/credstore`. Forces replacement " +
					"when changed, since flipping this switches the remote path (`/etc/credstore` vs " +
					"`/etc/credstore.encrypted`) and an in-place update would orphan the credential at the old path.",
				Default: booldefault.StaticBool(true),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"with_key": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Key source passed to `systemd-creds encrypt --with-key` " +
					"(`auto`, `host`, `tpm2`, or `host+tpm2`). Only meaningful when `encrypted = true`.",
				Validators: []validator.String{
					credentialKeySourceValidator(),
				},
			},
			"id": idAttribute(),
		},
	}
}

func (r *credentialResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg credentialModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.WithKey.IsNull() || cfg.WithKey.IsUnknown() {
		return
	}
	if cfg.Encrypted.IsUnknown() {
		return
	}
	if !cfg.Encrypted.IsNull() && !cfg.Encrypted.ValueBool() {
		resp.Diagnostics.AddAttributeError(
			path.Root("with_key"),
			"Invalid configuration",
			"with_key is only meaningful when encrypted = true",
		)
	}
}

func (r *credentialResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *credentialResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan credentialModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.PutCredential(ctx, plan.Name.ValueString(), plan.Data.ValueString(), plan.Encrypted.ValueBool(), plan.WithKey.ValueString()); err != nil {
		resp.Diagnostics.AddError("create credential", err.Error())
		return
	}
	plan.ID = plan.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *credentialResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state credentialModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	exists, err := r.client.HasCredential(ctx, state.Name.ValueString(), state.Encrypted.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("check credential", err.Error())
		return
	}
	if !exists {
		resp.State.RemoveResource(ctx)
		return
	}
	// The credential store never returns content back, so `data` (and every
	// other attribute) is kept as-is from state instead of refreshed.
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *credentialResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan credentialModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.PutCredential(ctx, plan.Name.ValueString(), plan.Data.ValueString(), plan.Encrypted.ValueBool(), plan.WithKey.ValueString()); err != nil {
		resp.Diagnostics.AddError("update credential", err.Error())
		return
	}
	plan.ID = plan.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *credentialResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state credentialModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteCredential(ctx, state.Name.ValueString(), state.Encrypted.ValueBool()); err != nil {
		resp.Diagnostics.AddError("delete credential", err.Error())
	}
}
