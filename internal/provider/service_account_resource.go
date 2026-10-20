package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"

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
	_ resource.Resource                = &ServiceAccountResource{}
	_ resource.ResourceWithConfigure   = &ServiceAccountResource{}
	_ resource.ResourceWithImportState = &ServiceAccountResource{}
)

func NewServiceAccountResource() resource.Resource {
	return &ServiceAccountResource{}
}

type ServiceAccountResource struct {
	client api.Client
}

type ServiceAccountModel struct {
	ID          types.String `tfsdk:"id"`
	IdentityID  types.String `tfsdk:"identity_id"`
	Name        types.String `tfsdk:"name"`
	DisplayName types.String `tfsdk:"display_name"`
	Description types.String `tfsdk:"description"`
}

func (r *ServiceAccountResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_account"
}

func (r *ServiceAccountResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages RWX service-account metadata without [issuing credentials](https://www.rwx.com/docs/service-accounts#access-tokens) or managing permissions. Use [rwx_service_account_permissions](https://registry.terraform.io/providers/rwx-cloud/rwx/latest/docs/resources/service_account_permissions) for direct permissions, [rwx_vault_service_account_attachment](https://registry.terraform.io/providers/rwx-cloud/rwx/latest/docs/resources/vault_service_account_attachment) for task-scoped token access, or [rwx_vault_service_account_access_grant](https://registry.terraform.io/providers/rwx-cloud/rwx/latest/docs/resources/vault_service_account_access_grant) for vault access. To reference an existing account by display name or ID, use the [rwx_service_account data source](https://registry.terraform.io/providers/rwx-cloud/rwx/latest/docs/data-sources/service_account). Import by the stable service-account ID. Renaming the display name preserves identity. Destroy deactivates the account, revokes its tokens, and ends its group memberships.\n\n## Required permissions\n\nThe provider's access token needs `service_account:create` to create service accounts and `service_account:manage` on the target account to update or deactivate it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The stable service-account ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"identity_id": schema.StringAttribute{
				Description: "The stable identity ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The service-account name returned by RWX for use in vault resources and run definitions.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"display_name": schema.StringAttribute{
				Description: "The display name, unique among active service accounts in the organization.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(regexp.MustCompile(`\S`), "must not be blank"),
				},
			},
			"description": schema.StringAttribute{
				Description: "The service-account description. Omit to clear the description.",
				Optional:    true,
			},
		},
	}
}

func (r *ServiceAccountResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ServiceAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ServiceAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	account, err := r.client.CreateServiceAccount(serviceAccountFromModel(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating service account in RWX", err.Error())
		return
	}
	state := serviceAccountModel(account)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ServiceAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ServiceAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	account, err := r.client.GetServiceAccount(state.ID.ValueString())
	if errors.Is(err, api.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading service account from RWX", err.Error())
		return
	}
	state = serviceAccountModel(account)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ServiceAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ServiceAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	account := serviceAccountFromModel(plan)
	account.ID = state.ID.ValueString()
	account, err := r.client.UpdateServiceAccount(account)
	if err != nil {
		resp.Diagnostics.AddError("Error updating service account in RWX", err.Error())
		return
	}
	state = serviceAccountModel(account)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ServiceAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ServiceAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteServiceAccount(state.ID.ValueString()); err != nil && !errors.Is(err, api.ErrNotFound) {
		resp.Diagnostics.AddError("Error deactivating service account in RWX", err.Error())
	}
}

func (r *ServiceAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func serviceAccountFromModel(model ServiceAccountModel) api.ServiceAccount {
	return api.ServiceAccount{
		ID:          model.ID.ValueString(),
		DisplayName: model.DisplayName.ValueString(),
		Description: model.Description.ValueStringPointer(),
	}
}

func serviceAccountModel(account api.ServiceAccount) ServiceAccountModel {
	return ServiceAccountModel{
		ID:          types.StringValue(account.ID),
		IdentityID:  types.StringValue(account.IdentityID),
		Name:        types.StringValue(account.Name),
		DisplayName: types.StringValue(account.DisplayName),
		Description: types.StringPointerValue(account.Description),
	}
}
