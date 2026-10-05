package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &VaultResource{}
	_ resource.ResourceWithConfigure   = &VaultResource{}
	_ resource.ResourceWithImportState = &VaultResource{}
)

var vaultRepositoryPermissionObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"repository_slug": types.StringType,
		"branch_pattern":  types.StringType,
	},
}

func NewVaultResource() resource.Resource {
	return &VaultResource{}
}

type VaultResource struct {
	client api.Client
}

type VaultResourceModel struct {
	ID                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
	Unlocked              types.Bool   `tfsdk:"unlocked"`
	ApprovalsEnabled      types.Bool   `tfsdk:"approvals_enabled"`
	RequiredApprovals     types.Int64  `tfsdk:"required_approvals"`
	RepositoryPermissions types.Set    `tfsdk:"repository_permissions"`
	OIDCSubject           types.String `tfsdk:"oidc_subject"`
}

type VaultRepositoryPermissionModel struct {
	RepositorySlug types.String `tfsdk:"repository_slug"`
	BranchPattern  types.String `tfsdk:"branch_pattern"`
}

func (r *VaultResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vault"
}

func (r *VaultResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Manages an RWX vault.",
		MarkdownDescription: "Manages an RWX vault. Learn more about [vaults](https://www.rwx.com/docs/vaults).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The stable ID of the vault.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the vault.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[a-zA-Z0-9_-]*$`),
						"can only include alphanumeric characters, dashes, or underscores",
					),
				},
			},
			"unlocked": schema.BoolAttribute{
				Description: "Whether the vault can be accessed without an explicit access grant.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"approvals_enabled": schema.BoolAttribute{
				Description: "Whether access to the vault requires approval.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"required_approvals": schema.Int64Attribute{
				Description: "The number of approvals required to access the vault.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(1),
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"repository_permissions": schema.SetNestedAttribute{
				Description: "The repositories and branches that can access the vault.",
				Optional:    true,
				Computed:    true,
				Default: setdefault.StaticValue(
					types.SetValueMust(vaultRepositoryPermissionObjectType, []attr.Value{}),
				),
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"repository_slug": schema.StringAttribute{
							Description: "The repository slug, such as rwx-cloud/cloud.",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
						"branch_pattern": schema.StringAttribute{
							Description: "The branch pattern allowed to access the vault.",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
					},
				},
			},
			"oidc_subject": schema.StringAttribute{
				Description: "The stable OIDC subject associated with the vault.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *VaultResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *VaultResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan VaultResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vault, diags := vaultFromResourceModel(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	vault, err := r.client.CreateVault(vault)
	if err != nil {
		if errors.Is(err, api.ErrConflict) {
			resp.Diagnostics.AddError(
				"Vault already exists",
				fmt.Sprintf("RWX already contains a vault named %q. Import the existing vault by its ID or choose a different name.", plan.Name.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError("Error creating vault in RWX", "Unexpected error: "+err.Error())
		return
	}

	state, diags := vaultResourceModel(ctx, vault)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *VaultResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state VaultResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vault, err := r.client.GetVault(state.ID.ValueString())
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading vault from RWX", "Unexpected error: "+err.Error())
		return
	}

	state, diags := vaultResourceModel(ctx, vault)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *VaultResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan VaultResourceModel
	var state VaultResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vault, diags := vaultFromResourceModel(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	vault.ID = state.ID.ValueString()

	vault, err := r.client.UpdateVault(vault)
	if err != nil {
		if errors.Is(err, api.ErrConflict) {
			resp.Diagnostics.AddError("Vault update conflicts with existing RWX configuration", err.Error())
			return
		}
		resp.Diagnostics.AddError("Error updating vault in RWX", "Unexpected error: "+err.Error())
		return
	}

	state, diags = vaultResourceModel(ctx, vault)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *VaultResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state VaultResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteVault(state.ID.ValueString())
	if err != nil && !errors.Is(err, api.ErrNotFound) {
		resp.Diagnostics.AddError("Error deleting vault from RWX", "RWX rejected the vault deletion: "+err.Error())
	}
}

func (r *VaultResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func vaultFromResourceModel(ctx context.Context, model VaultResourceModel) (api.Vault, diag.Diagnostics) {
	var permissions []VaultRepositoryPermissionModel
	diags := model.RepositoryPermissions.ElementsAs(ctx, &permissions, false)

	apiPermissions := make([]api.VaultRepositoryPermission, 0, len(permissions))
	for _, permission := range permissions {
		apiPermissions = append(apiPermissions, api.VaultRepositoryPermission{
			RepositorySlug: permission.RepositorySlug.ValueString(),
			BranchPattern:  permission.BranchPattern.ValueString(),
		})
	}

	lockStatus := "locked"
	if model.Unlocked.ValueBool() {
		lockStatus = "unlocked"
	}

	return api.Vault{
		ID:                    model.ID.ValueString(),
		Name:                  model.Name.ValueString(),
		LockStatus:            lockStatus,
		ApprovalsEnabled:      model.ApprovalsEnabled.ValueBool(),
		RequiredApprovals:     model.RequiredApprovals.ValueInt64(),
		RepositoryPermissions: apiPermissions,
		OIDCSubject:           model.OIDCSubject.ValueString(),
	}, diags
}

func vaultResourceModel(ctx context.Context, vault api.Vault) (VaultResourceModel, diag.Diagnostics) {
	permissions := make([]VaultRepositoryPermissionModel, 0, len(vault.RepositoryPermissions))
	for _, permission := range vault.RepositoryPermissions {
		permissions = append(permissions, VaultRepositoryPermissionModel{
			RepositorySlug: types.StringValue(permission.RepositorySlug),
			BranchPattern:  types.StringValue(permission.BranchPattern),
		})
	}

	permissionSet, diags := types.SetValueFrom(ctx, vaultRepositoryPermissionObjectType, permissions)
	return VaultResourceModel{
		ID:                    types.StringValue(vault.ID),
		Name:                  types.StringValue(vault.Name),
		Unlocked:              types.BoolValue(vault.LockStatus == "unlocked"),
		ApprovalsEnabled:      types.BoolValue(vault.ApprovalsEnabled),
		RequiredApprovals:     types.Int64Value(vault.RequiredApprovals),
		RepositoryPermissions: permissionSet,
		OIDCSubject:           types.StringValue(vault.OIDCSubject),
	}, diags
}
