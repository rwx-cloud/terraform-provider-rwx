package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &accessGrantResource{}
	_ resource.ResourceWithConfigure   = &accessGrantResource{}
	_ resource.ResourceWithImportState = &accessGrantResource{}
	_ resource.ResourceWithModifyPlan  = &accessGrantResource{}
)

type accessGrantResource struct {
	client        api.Client
	principalType string
}

type accessGrantResourceModel struct {
	ID        types.String
	Vault     types.String
	VaultID   types.String
	Principal types.String
	ExpiresAt types.String
}

type userAccessGrantResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Vault     types.String `tfsdk:"vault"`
	VaultID   types.String `tfsdk:"vault_id"`
	Email     types.String `tfsdk:"email"`
	ExpiresAt types.String `tfsdk:"expires_at"`
}

type serviceAccountAccessGrantResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Vault          types.String `tfsdk:"vault"`
	VaultID        types.String `tfsdk:"vault_id"`
	ServiceAccount types.String `tfsdk:"service_account"`
	ExpiresAt      types.String `tfsdk:"expires_at"`
}

func NewVaultUserAccessGrantResource() resource.Resource {
	return &accessGrantResource{principalType: api.AccessGrantPrincipalTypeUser}
}

func NewVaultServiceAccountAccessGrantResource() resource.Resource {
	return &accessGrantResource{principalType: api.AccessGrantPrincipalTypeServiceAccount}
}

func (r *accessGrantResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vault_" + r.principalType + "_access_grant"
}

func (r *accessGrantResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	principalName := "email"
	principalDescription := "The email address of the RWX user that can access the vault."
	if r.principalType == api.AccessGrantPrincipalTypeServiceAccount {
		principalName = "service_account"
		principalDescription = "The name of the RWX service account that can access the vault."
	}

	resp.Schema = schema.Schema{
		Description: "Manages access to a locked RWX vault for an RWX " + r.principalLabel() + ".",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The stable ID of the access grant.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"vault": vaultNameAttribute("The name of the RWX vault to grant access to."),
			"vault_id": vaultIDAttribute(
				"The stable ID of the RWX vault to grant access to. Reference rwx_vault.<name>.id for managed vaults.",
			),
			principalName: schema.StringAttribute{
				Description: principalDescription,
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"expires_at": schema.StringAttribute{
				Description: "The optional ISO 8601 timestamp when access expires. Omit this attribute for permanent access.",
				Optional:    true,
			},
		},
	}
}

func (r *accessGrantResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *accessGrantResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	state := r.readModel(ctx, &req.State, &resp.Diagnostics)
	plan := r.readModel(ctx, &req.Plan, &resp.Diagnostics)
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

func (r *accessGrantResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	plan := r.readModel(ctx, &req.Plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	vaultID, selectorDiags := resolveVaultID(r.client, plan.vaultSelector(), "")
	resp.Diagnostics.Append(selectorDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	grants, err := r.client.ListAccessGrants(vaultID)
	if err != nil {
		resp.Diagnostics.AddError("Error checking access grants in RWX", "Unexpected error: "+err.Error())
		return
	}
	for _, grant := range grants {
		if r.matchesPrincipal(grant.Principal, plan.Principal.ValueString()) {
			resp.Diagnostics.AddError(
				"Access grant already exists",
				fmt.Sprintf("Vault %q already contains an access grant for %s %q. Import the existing grant with ID %q or choose a different principal.", vaultID, r.principalLabel(), plan.Principal.ValueString(), grant.ID),
			)
			return
		}
	}

	grant, err := r.client.CreateAccessGrant(vaultID, r.apiPrincipal(plan.Principal.ValueString()), expirationPointer(plan.ExpiresAt))
	if err != nil {
		resp.Diagnostics.AddError("Error creating access grant in RWX", "Unexpected error: "+err.Error())
		return
	}

	state := r.resourceModel(grant, plan, plan.ExpiresAt)
	resp.Diagnostics.Append(storeAccessGrantVaultID(ctx, grant.Vault.ID, resp.Private)...)
	r.writeModel(ctx, &resp.State, state, &resp.Diagnostics)
}

func (r *accessGrantResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	state := r.readModel(ctx, &req.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	privateValue, diags := req.Private.GetKey(ctx, "vault_id")
	resp.Diagnostics.Append(diags...)
	previousID := privateVaultID(privateValue, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	var grant api.AccessGrant
	var err error
	if previousID == "" && configuredVaultSelector(state.vaultSelector()) == (api.VaultSelector{}) {
		grant, err = r.client.FindAccessGrant(state.ID.ValueString())
	} else {
		var vaultID string
		vaultID, diags = resolveVaultID(r.client, state.vaultSelector(), previousID)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		grant, err = r.client.GetAccessGrant(vaultID, state.ID.ValueString())
	}
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading access grant from RWX", "Unexpected error: "+err.Error())
		return
	}
	if grant.Principal.Type != r.principalType {
		resp.Diagnostics.AddError(
			"Access grant type does not match resource",
			fmt.Sprintf("Access grant %q belongs to a %s, but this resource manages %ss.", grant.ID, principalTypeLabel(grant.Principal.Type), r.principalLabel()),
		)
		return
	}

	state = r.resourceModel(grant, state, equivalentExpiration(grant.ExpiresAt, state.ExpiresAt))
	resp.Diagnostics.Append(storeAccessGrantVaultID(ctx, grant.Vault.ID, resp.Private)...)
	r.writeModel(ctx, &resp.State, state, &resp.Diagnostics)
}

func (r *accessGrantResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	plan := r.readModel(ctx, &req.Plan, &resp.Diagnostics)
	state := r.readModel(ctx, &req.State, &resp.Diagnostics)
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

	grant, err := r.client.UpdateAccessGrant(vaultID, state.ID.ValueString(), expirationPointer(plan.ExpiresAt))
	if err != nil {
		resp.Diagnostics.AddError("Error updating access grant in RWX", "Unexpected error: "+err.Error())
		return
	}

	state = r.resourceModel(grant, plan, plan.ExpiresAt)
	resp.Diagnostics.Append(storeAccessGrantVaultID(ctx, grant.Vault.ID, resp.Private)...)
	r.writeModel(ctx, &resp.State, state, &resp.Diagnostics)
}

func (r *accessGrantResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	state := r.readModel(ctx, &req.State, &resp.Diagnostics)
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

	if err := r.client.DeleteAccessGrant(vaultID, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting access grant from RWX", "Unexpected error: "+err.Error())
	}
}

func (r *accessGrantResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

type accessGrantModelReader interface {
	Get(context.Context, any) diag.Diagnostics
}

type accessGrantModelWriter interface {
	Set(context.Context, any) diag.Diagnostics
}

type accessGrantPrivateWriter interface {
	SetKey(context.Context, string, []byte) diag.Diagnostics
}

func (r *accessGrantResource) readModel(ctx context.Context, reader accessGrantModelReader, diags *diag.Diagnostics) accessGrantResourceModel {
	if r.principalType == api.AccessGrantPrincipalTypeUser {
		var model userAccessGrantResourceModel
		diags.Append(reader.Get(ctx, &model)...)
		return accessGrantResourceModel{ID: model.ID, Vault: model.Vault, VaultID: model.VaultID, Principal: model.Email, ExpiresAt: model.ExpiresAt}
	}

	var model serviceAccountAccessGrantResourceModel
	diags.Append(reader.Get(ctx, &model)...)
	return accessGrantResourceModel{ID: model.ID, Vault: model.Vault, VaultID: model.VaultID, Principal: model.ServiceAccount, ExpiresAt: model.ExpiresAt}
}

func (r *accessGrantResource) writeModel(ctx context.Context, writer accessGrantModelWriter, model accessGrantResourceModel, diags *diag.Diagnostics) {
	var value any
	if r.principalType == api.AccessGrantPrincipalTypeUser {
		value = userAccessGrantResourceModel{ID: model.ID, Vault: model.Vault, VaultID: model.VaultID, Email: model.Principal, ExpiresAt: model.ExpiresAt}
	} else {
		value = serviceAccountAccessGrantResourceModel{ID: model.ID, Vault: model.Vault, VaultID: model.VaultID, ServiceAccount: model.Principal, ExpiresAt: model.ExpiresAt}
	}
	diags.Append(writer.Set(ctx, value)...)
}

func (r *accessGrantResource) resourceModel(grant api.AccessGrant, configured accessGrantResourceModel, expiresAt types.String) accessGrantResourceModel {
	if configured.Vault.IsNull() && configured.VaultID.IsNull() {
		configured.Vault = types.StringValue(grant.Vault.Name)
	}
	principal := configured.Principal
	if principal.IsNull() || principal.IsUnknown() {
		principal = types.StringValue(r.principalValue(grant.Principal))
	}

	return accessGrantResourceModel{
		ID:        types.StringValue(grant.ID),
		Vault:     configured.Vault,
		VaultID:   configured.VaultID,
		Principal: principal,
		ExpiresAt: expiresAt,
	}
}

func (r *accessGrantResource) apiPrincipal(value string) api.AccessGrantPrincipal {
	principal := api.AccessGrantPrincipal{Type: r.principalType}
	if r.principalType == api.AccessGrantPrincipalTypeUser {
		principal.Email = value
	} else {
		principal.Name = value
	}
	return principal
}

func (r *accessGrantResource) principalValue(principal api.AccessGrantPrincipal) string {
	if r.principalType == api.AccessGrantPrincipalTypeUser {
		return principal.Email
	}
	return principal.Name
}

func (r *accessGrantResource) matchesPrincipal(principal api.AccessGrantPrincipal, value string) bool {
	if principal.Type != r.principalType {
		return false
	}
	if r.principalType == api.AccessGrantPrincipalTypeUser {
		return strings.EqualFold(principal.Email, value)
	}
	return principal.Name == value
}

func (r *accessGrantResource) principalLabel() string {
	return principalTypeLabel(r.principalType)
}

func principalTypeLabel(principalType string) string {
	if principalType == api.AccessGrantPrincipalTypeServiceAccount {
		return "service account"
	}
	return "user"
}

func (m accessGrantResourceModel) vaultSelector() vaultSelectorModel {
	return vaultSelectorModel{vault: m.Vault, vaultID: m.VaultID}
}

func expirationPointer(value types.String) *string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	expiresAt := value.ValueString()
	return &expiresAt
}

func equivalentExpiration(expiresAt *string, previous types.String) types.String {
	if expiresAt == nil {
		return types.StringNull()
	}
	if !previous.IsNull() && !previous.IsUnknown() {
		actual, actualErr := time.Parse(time.RFC3339, *expiresAt)
		prior, priorErr := time.Parse(time.RFC3339, previous.ValueString())
		if actualErr == nil && priorErr == nil && actual.Equal(prior) {
			return previous
		}
	}
	return types.StringValue(*expiresAt)
}

func storeAccessGrantVaultID(ctx context.Context, vaultID string, private accessGrantPrivateWriter) diag.Diagnostics {
	var diags diag.Diagnostics
	encodedVaultID, err := encodePrivateVaultID(vaultID)
	if err != nil {
		diags.AddError("Error storing access grant state", "Unexpected error: "+err.Error())
		return diags
	}
	diags.Append(private.SetKey(ctx, "vault_id", encodedVaultID)...)
	return diags
}
