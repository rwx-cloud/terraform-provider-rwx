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
	_ resource.Resource                = &GroupPermissionsResource{}
	_ resource.ResourceWithConfigure   = &GroupPermissionsResource{}
	_ resource.ResourceWithImportState = &GroupPermissionsResource{}
	_ resource.ResourceWithModifyPlan  = &GroupPermissionsResource{}
)

func NewGroupPermissionsResource() resource.Resource {
	return &GroupPermissionsResource{}
}

type GroupPermissionsResource struct {
	client api.Client
}

type GroupPermissionsModel struct {
	ID          types.String `tfsdk:"id"`
	GroupID     types.String `tfsdk:"group_id"`
	Permissions types.Set    `tfsdk:"permissions"`
}

func (r *GroupPermissionsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_permissions"
}

func (r *GroupPermissionsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the complete direct-permission set of an RWX group. Use only one resource per group. Apply replaces all direct assignments, including changes made in the UI; inherited permissions are untouched. Set `permissions = []` to clear direct assignments. Destroy clears direct permissions without deleting the group. Changing `group_id` replaces this resource and clears the old group's direct permissions. Planning validates known permission keys, scopes, and grantability using the RWX catalog; checks requiring unknown target or resource IDs are deferred until those IDs are known. RWX rechecks authority when applying and rejects unauthorized changes and changes to protected groups. Import by the group ID.\n\n## Required permissions\n\nThe provider's access token needs `group:manage` on the target group for targeted planning and writes. It must also have authority to grant or remove each changed permission at its requested scope. These token requirements are separate from the assignments in `permissions`. See [permissions](https://www.rwx.com/docs/permissions).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The group ID.",
				Computed:    true,
			},
			"group_id": schema.StringAttribute{
				Description:   "The stable ID of the group whose direct permissions are managed.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`\S`), "must not be blank")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"permissions": permissionsAttribute(),
		},
	}
}

func (r *GroupPermissionsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GroupPermissionsResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var plan GroupPermissionsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("id"), plan.GroupID)...)
	if resp.Diagnostics.HasError() || r.client.RoundTrip == nil {
		return
	}
	targetKnown := !plan.GroupID.IsNull() && !plan.GroupID.IsUnknown()
	target := url.Values{}
	var current []api.Permission
	if targetKnown {
		target.Set("group_id", plan.GroupID.ValueString())
		var err error
		current, err = r.client.GetGroupPermissions(plan.GroupID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error reading group permissions from RWX", err.Error())
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

func (r *GroupPermissionsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan GroupPermissionsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	permissions, diags := permissionsFromModel(ctx, plan.Permissions)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	permissions, err := r.client.ReplaceGroupPermissions(plan.GroupID.ValueString(), permissions)
	if err != nil {
		resp.Diagnostics.AddError("Error replacing group permissions in RWX", err.Error())
		return
	}
	plan.ID = plan.GroupID
	plan.Permissions, diags = permissionsModel(ctx, permissions)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *GroupPermissionsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state GroupPermissionsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	permissions, err := r.client.GetGroupPermissions(state.ID.ValueString())
	if errors.Is(err, api.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading group permissions from RWX", err.Error())
		return
	}
	if err := r.client.ClaimTerraformManagement(ctx, "/mint/api/groups/"+url.PathEscape(state.ID.ValueString())+"/permissions"); err != nil {
		resp.Diagnostics.AddError("Error claiming Terraform management", err.Error()+". Verify that the access token can manage these group permissions, then retry the refresh.")
		return
	}
	state.GroupID = state.ID
	value, diags := permissionsModel(ctx, permissions)
	state.Permissions = value
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *GroupPermissionsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan GroupPermissionsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	permissions, diags := permissionsFromModel(ctx, plan.Permissions)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	permissions, err := r.client.ReplaceGroupPermissions(plan.GroupID.ValueString(), permissions)
	if err != nil {
		resp.Diagnostics.AddError("Error replacing group permissions in RWX", err.Error())
		return
	}
	plan.ID = plan.GroupID
	plan.Permissions, diags = permissionsModel(ctx, permissions)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *GroupPermissionsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state GroupPermissionsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.ClearGroupPermissions(state.ID.ValueString()); err != nil && !errors.Is(err, api.ErrNotFound) {
		resp.Diagnostics.AddError("Error clearing group permissions in RWX", err.Error())
	}
}

func (r *GroupPermissionsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
