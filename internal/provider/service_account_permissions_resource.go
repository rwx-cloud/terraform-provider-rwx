package provider

import (
	"context"
	"errors"
	"fmt"
	"net/url"
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
	_ resource.Resource                = &ServiceAccountPermissionsResource{}
	_ resource.ResourceWithConfigure   = &ServiceAccountPermissionsResource{}
	_ resource.ResourceWithImportState = &ServiceAccountPermissionsResource{}
	_ resource.ResourceWithModifyPlan  = &ServiceAccountPermissionsResource{}
)

func NewServiceAccountPermissionsResource() resource.Resource {
	return &ServiceAccountPermissionsResource{}
}

type ServiceAccountPermissionsResource struct {
	client api.Client
}

type ServiceAccountPermissionsModel struct {
	ID               types.String `tfsdk:"id"`
	ServiceAccountID types.String `tfsdk:"service_account_id"`
	Permissions      types.Set    `tfsdk:"permissions"`
}

func (r *ServiceAccountPermissionsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_account_permissions"
}

func (r *ServiceAccountPermissionsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the complete direct-permission set of an RWX service account, separately from its metadata and credentials. Inherited group access is unchanged. Changes made outside Terraform are restored on apply. Destroy clears direct permissions without deactivating the account. Manage each account's direct permissions with only one resource. Import by service-account ID.\n\nKnown keys, scopes, and acting-token authority are checked against the permission catalog during planning. Checks requiring unknown same-plan IDs are deferred. RWX rechecks authority on apply and rejects an invalid replacement without partial writes.\n\n## Required permissions\n\nThe provider's access token needs `service_account:manage` on the target account for targeted planning and writes. It must also have authority to grant or remove each changed permission at its requested scope. These token requirements are separate from the assignments in `permissions`. See [permissions](https://www.rwx.com/docs/permissions).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The service-account ID. Use this ID to import existing direct permissions.",
				Computed:    true,
			},
			"service_account_id": schema.StringAttribute{
				Description:   "The service account whose direct permissions are managed. Changing this ID replaces the resource and clears the old account's direct permissions.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`\S`), "must not be blank")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"permissions": permissionsAttribute(),
		},
	}
}

func (r *ServiceAccountPermissionsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ServiceAccountPermissionsResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var plan ServiceAccountPermissionsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("id"), plan.ServiceAccountID)...)
	if resp.Diagnostics.HasError() || r.client.RoundTrip == nil {
		return
	}
	targetKnown := !plan.ServiceAccountID.IsNull() && !plan.ServiceAccountID.IsUnknown()
	target := url.Values{}
	var current []api.Permission
	if targetKnown {
		target.Set("service_account_id", plan.ServiceAccountID.ValueString())
		var err error
		current, err = r.client.GetServiceAccountPermissions(plan.ServiceAccountID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error reading service account permissions from RWX", err.Error())
			return
		}
	}
	catalog, err := r.client.GetPermissionCatalog(target)
	if err != nil {
		resp.Diagnostics.AddError("Error reading permission catalog from RWX", err.Error())
		return
	}
	resp.Diagnostics.Append(validatePermissions(ctx, plan.Permissions, catalog, current, targetKnown)...)
}

func (r *ServiceAccountPermissionsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ServiceAccountPermissionsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	permissions, diags := permissionsFromModel(ctx, plan.Permissions)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	permissions, err := r.client.ReplaceServiceAccountPermissions(plan.ServiceAccountID.ValueString(), permissions)
	if err != nil {
		resp.Diagnostics.AddError("Error replacing service account permissions in RWX", err.Error())
		return
	}
	plan.ID = plan.ServiceAccountID
	plan.Permissions, diags = permissionsModel(ctx, permissions)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServiceAccountPermissionsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ServiceAccountPermissionsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	permissions, err := r.client.GetServiceAccountPermissions(state.ID.ValueString())
	if errors.Is(err, api.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading service account permissions from RWX", err.Error())
		return
	}
	state.ServiceAccountID = state.ID
	value, diags := permissionsModel(ctx, permissions)
	state.Permissions = value
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ServiceAccountPermissionsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ServiceAccountPermissionsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	permissions, diags := permissionsFromModel(ctx, plan.Permissions)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	permissions, err := r.client.ReplaceServiceAccountPermissions(plan.ServiceAccountID.ValueString(), permissions)
	if err != nil {
		resp.Diagnostics.AddError("Error replacing service account permissions in RWX", err.Error())
		return
	}
	plan.ID = plan.ServiceAccountID
	plan.Permissions, diags = permissionsModel(ctx, permissions)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServiceAccountPermissionsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ServiceAccountPermissionsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.ReplaceServiceAccountPermissions(state.ID.ValueString(), []api.Permission{}); err != nil && !errors.Is(err, api.ErrNotFound) {
		resp.Diagnostics.AddError("Error clearing service account permissions in RWX", err.Error())
	}
}

func (r *ServiceAccountPermissionsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
