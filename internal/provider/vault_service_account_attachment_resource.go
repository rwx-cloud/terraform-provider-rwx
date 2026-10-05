package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &VaultServiceAccountAttachmentResource{}
	_ resource.ResourceWithConfigure   = &VaultServiceAccountAttachmentResource{}
	_ resource.ResourceWithImportState = &VaultServiceAccountAttachmentResource{}
	_ resource.ResourceWithModifyPlan  = &VaultServiceAccountAttachmentResource{}
)

func NewVaultServiceAccountAttachmentResource() resource.Resource {
	return &VaultServiceAccountAttachmentResource{}
}

type VaultServiceAccountAttachmentResource struct {
	client api.Client
}

type VaultServiceAccountAttachmentResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Vault            types.String `tfsdk:"vault"`
	VaultID          types.String `tfsdk:"vault_id"`
	ServiceAccount   types.String `tfsdk:"service_account"`
	ServiceAccountID types.String `tfsdk:"service_account_id"`
	CreatedAt        types.String `tfsdk:"created_at"`
}

func (r *VaultServiceAccountAttachmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vault_service_account_attachment"
}

func (r *VaultServiceAccountAttachmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Attaches a service account to an RWX vault for task-scoped token access.",
		MarkdownDescription: "Attaches a service account to an RWX vault for task-scoped token access. Learn more about [assuming a service account in a task](https://www.rwx.com/docs/service-accounts#assuming-a-service-account-in-a-task).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The stable ID of the service-account attachment.",
				Computed:    true,
			},
			"vault": vaultNameAttribute("The name of the RWX vault for this service-account attachment."),
			"vault_id": vaultIDAttribute(
				"The stable ID of the RWX vault for this service-account attachment. Reference rwx_vault.<name>.id for managed vaults.",
			),
			"service_account": schema.StringAttribute{
				Description: "The name of the service account attached to the vault.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"service_account_id": schema.StringAttribute{
				Description: "The stable ID of the RWX service account.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "The time when the service account was attached to the vault.",
				Computed:    true,
			},
		},
	}
}

func (r *VaultServiceAccountAttachmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(api.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected api.Client, got: %T. Please report this issue to support@rwx.com.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *VaultServiceAccountAttachmentResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var state VaultServiceAccountAttachmentResourceModel
	var plan VaultServiceAccountAttachmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	privateValue, diags := req.Private.GetKey(ctx, "vault_id")
	resp.Diagnostics.Append(diags...)
	previousID := privateVaultID(privateValue, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	requiresReplace, selectorDiags := vaultSelectorsRequireReplace(r.client, state.vaultSelector(), plan.vaultSelector(), previousID)
	resp.Diagnostics.Append(selectorDiags...)
	if requiresReplace {
		resp.RequiresReplace = append(resp.RequiresReplace, vaultReplacementPath(plan.vaultSelector()))
	}
}

func (r *VaultServiceAccountAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan VaultServiceAccountAttachmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vaultID, diags := resolveVaultID(r.client, plan.vaultSelector(), "")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	attachment, err := r.client.CreateVaultServiceAccountAttachment(vaultID, plan.ServiceAccount.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating service-account attachment in RWX", "Unexpected error: "+err.Error())
		return
	}

	state := vaultServiceAccountAttachmentResourceModel(attachment, plan.vaultSelector())
	encodedVaultID, err := encodePrivateVaultID(attachment.Vault.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error storing service-account attachment state", "Unexpected error: "+err.Error())
		return
	}
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, "vault_id", encodedVaultID)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *VaultServiceAccountAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state VaultServiceAccountAttachmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	privateValue, diags := req.Private.GetKey(ctx, "vault_id")
	resp.Diagnostics.Append(diags...)
	previousID := privateVaultID(privateValue, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	vaultID, selectorDiags := resolveVaultID(r.client, state.vaultSelector(), previousID)
	resp.Diagnostics.Append(selectorDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	attachment, err := r.client.GetVaultServiceAccountAttachment(vaultID, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading service-account attachment from RWX", "Unexpected error: "+err.Error())
		return
	}

	state = vaultServiceAccountAttachmentResourceModel(attachment, state.vaultSelector())
	encodedVaultID, err := encodePrivateVaultID(attachment.Vault.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error storing service-account attachment state", "Unexpected error: "+err.Error())
		return
	}
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, "vault_id", encodedVaultID)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *VaultServiceAccountAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan VaultServiceAccountAttachmentResourceModel
	var state VaultServiceAccountAttachmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	privateValue, diags := req.Private.GetKey(ctx, "vault_id")
	resp.Diagnostics.Append(diags...)
	previousID := privateVaultID(privateValue, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	vaultID, selectorDiags := resolveVaultID(r.client, plan.vaultSelector(), previousID)
	resp.Diagnostics.Append(selectorDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.Vault = plan.Vault
	state.VaultID = plan.VaultID
	encodedVaultID, err := encodePrivateVaultID(vaultID)
	if err != nil {
		resp.Diagnostics.AddError("Error storing service-account attachment state", "Unexpected error: "+err.Error())
		return
	}
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, "vault_id", encodedVaultID)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *VaultServiceAccountAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state VaultServiceAccountAttachmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	privateValue, diags := req.Private.GetKey(ctx, "vault_id")
	resp.Diagnostics.Append(diags...)
	previousID := privateVaultID(privateValue, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	vaultID, selectorDiags := resolveVaultID(r.client, state.vaultSelector(), previousID)
	resp.Diagnostics.Append(selectorDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteVaultServiceAccountAttachment(vaultID, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting service-account attachment from RWX", "Unexpected error: "+err.Error())
	}
}

func (r *VaultServiceAccountAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	vaultID, attachmentID, ok := vaultRelationshipImportID(req.ID)
	if !ok {
		resp.Diagnostics.AddError("Invalid service-account attachment import ID", "Use vault-id/attachment-id.")
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vault_id"), vaultID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), attachmentID)...)
}

func vaultServiceAccountAttachmentResourceModel(attachment api.VaultServiceAccountAttachment, selector vaultSelectorModel) VaultServiceAccountAttachmentResourceModel {
	return VaultServiceAccountAttachmentResourceModel{
		ID:               types.StringValue(attachment.ID),
		Vault:            selector.vault,
		VaultID:          selector.vaultID,
		ServiceAccount:   types.StringValue(attachment.ServiceAccount.Name),
		ServiceAccountID: types.StringValue(attachment.ServiceAccount.ID),
		CreatedAt:        types.StringValue(attachment.CreatedAt),
	}
}

func (m VaultServiceAccountAttachmentResourceModel) vaultSelector() vaultSelectorModel {
	return vaultSelectorModel{vault: m.Vault, vaultID: m.VaultID}
}
