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
	_ resource.Resource                = &VaultApproverResource{}
	_ resource.ResourceWithConfigure   = &VaultApproverResource{}
	_ resource.ResourceWithImportState = &VaultApproverResource{}
	_ resource.ResourceWithModifyPlan  = &VaultApproverResource{}
)

func NewVaultApproverResource() resource.Resource {
	return &VaultApproverResource{}
}

type VaultApproverResource struct {
	client api.Client
}

type VaultApproverResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Vault   types.String `tfsdk:"vault"`
	VaultID types.String `tfsdk:"vault_id"`
	Email   types.String `tfsdk:"email"`
	UserID  types.String `tfsdk:"user_id"`
}

func (r *VaultApproverResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vault_approver"
}

func (r *VaultApproverResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Manages a user who can approve access to an RWX vault.",
		MarkdownDescription: "Manages a user who can approve access to an RWX vault. Learn more about [vault approvals](https://www.rwx.com/docs/vaults#approvals).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The stable ID of the vault approver.",
				Computed:    true,
			},
			"vault": vaultNameAttribute("The name of the RWX vault for this approver."),
			"vault_id": vaultIDAttribute(
				"The stable ID of the RWX vault for this approver. Reference rwx_vault.<name>.id for managed vaults.",
			),
			"email": schema.StringAttribute{
				Description: "The email address of the user who can approve vault access.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"user_id": schema.StringAttribute{
				Description: "The stable ID of the RWX user.",
				Computed:    true,
			},
		},
	}
}

func (r *VaultApproverResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *VaultApproverResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var state VaultApproverResourceModel
	var plan VaultApproverResourceModel
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

func (r *VaultApproverResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan VaultApproverResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vaultID, diags := resolveVaultID(r.client, plan.vaultSelector(), "")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	approver, err := r.client.CreateVaultApprover(vaultID, plan.Email.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating vault approver in RWX", "Unexpected error: "+err.Error())
		return
	}

	state := vaultApproverResourceModel(approver, plan.vaultSelector())
	encodedVaultID, err := encodePrivateVaultID(approver.Vault.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error storing vault approver state", "Unexpected error: "+err.Error())
		return
	}
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, "vault_id", encodedVaultID)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *VaultApproverResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state VaultApproverResourceModel
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

	approver, err := r.client.GetVaultApprover(vaultID, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading vault approver from RWX", "Unexpected error: "+err.Error())
		return
	}

	state = vaultApproverResourceModel(approver, state.vaultSelector())
	encodedVaultID, err := encodePrivateVaultID(approver.Vault.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error storing vault approver state", "Unexpected error: "+err.Error())
		return
	}
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, "vault_id", encodedVaultID)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *VaultApproverResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan VaultApproverResourceModel
	var state VaultApproverResourceModel
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
		resp.Diagnostics.AddError("Error storing vault approver state", "Unexpected error: "+err.Error())
		return
	}
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, "vault_id", encodedVaultID)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *VaultApproverResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state VaultApproverResourceModel
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

	if err := r.client.DeleteVaultApprover(vaultID, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting vault approver from RWX", "Unexpected error: "+err.Error())
	}
}

func (r *VaultApproverResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	vaultID, approverID, ok := vaultRelationshipImportID(req.ID)
	if !ok {
		resp.Diagnostics.AddError("Invalid vault approver import ID", "Use vault-id/approver-id.")
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vault_id"), vaultID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), approverID)...)
}

func vaultApproverResourceModel(approver api.VaultApprover, selector vaultSelectorModel) VaultApproverResourceModel {
	return VaultApproverResourceModel{
		ID:      types.StringValue(approver.ID),
		Vault:   selector.vault,
		VaultID: selector.vaultID,
		Email:   types.StringValue(approver.User.Email),
		UserID:  types.StringValue(approver.User.ID),
	}
}

func (m VaultApproverResourceModel) vaultSelector() vaultSelectorModel {
	return vaultSelectorModel{vault: m.Vault, vaultID: m.VaultID}
}
