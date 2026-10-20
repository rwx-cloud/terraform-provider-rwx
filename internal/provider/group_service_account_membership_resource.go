package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"
)

var (
	_ resource.Resource                = &GroupServiceAccountMembershipResource{}
	_ resource.ResourceWithConfigure   = &GroupServiceAccountMembershipResource{}
	_ resource.ResourceWithImportState = &GroupServiceAccountMembershipResource{}
)

func NewGroupServiceAccountMembershipResource() resource.Resource {
	return &GroupServiceAccountMembershipResource{}
}

type GroupServiceAccountMembershipResource struct {
	client api.Client
}

type GroupServiceAccountMembershipResourceModel struct {
	ID               types.String `tfsdk:"id"`
	GroupID          types.String `tfsdk:"group_id"`
	ServiceAccountID types.String `tfsdk:"service_account_id"`
}

func (r *GroupServiceAccountMembershipResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_service_account_membership"
}

func (r *GroupServiceAccountMembershipResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one direct service-account membership in a group. The service account inherits the group's permissions. See [the docs](https://www.rwx.com/docs/service-accounts#permissions) for more information on how service accounts and groups work together. Requires `org:access` to read and `group:manage` on the group to add or remove membership; `service_account:manage` is not required. Server authorization and safety checks apply.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The membership identity, in group-id/service-account-id format.",
				Computed:    true,
			},
			"group_id": schema.StringAttribute{
				Description:   "The ID of the group to join. Changing this replaces the membership.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"service_account_id": schema.StringAttribute{
				Description:   "The ID of the service account joining the group. Changing this replaces the membership.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
		},
	}
}

func (r *GroupServiceAccountMembershipResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(api.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected api.Client, got: %T. Please report this issue to support@rwx.com.", req.ProviderData))
		return
	}
	r.client = client
}

func (r *GroupServiceAccountMembershipResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan GroupServiceAccountMembershipResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	membership, err := r.client.CreateGroupServiceAccountMembership(plan.GroupID.ValueString(), plan.ServiceAccountID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating group service account membership in RWX", err.Error())
		return
	}
	state := groupServiceAccountMembershipResourceModel(membership)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *GroupServiceAccountMembershipResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state GroupServiceAccountMembershipResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	membership, err := r.client.GetGroupServiceAccountMembership(state.GroupID.ValueString(), state.ServiceAccountID.ValueString())
	if errors.Is(err, api.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading group service account membership from RWX", err.Error())
		return
	}
	state = groupServiceAccountMembershipResourceModel(membership)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *GroupServiceAccountMembershipResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Unexpected group service account membership update", "Changing either ID requires replacing the membership.")
}

func (r *GroupServiceAccountMembershipResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state GroupServiceAccountMembershipResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteGroupServiceAccountMembership(state.GroupID.ValueString(), state.ServiceAccountID.ValueString())
	if err != nil && !errors.Is(err, api.ErrNotFound) {
		resp.Diagnostics.AddError("Error deleting group service account membership from RWX", err.Error())
	}
}

func (r *GroupServiceAccountMembershipResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid group service account membership import ID", "Use group-id/service-account-id.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service_account_id"), parts[1])...)
}

func groupServiceAccountMembershipResourceModel(membership api.GroupServiceAccountMembership) GroupServiceAccountMembershipResourceModel {
	return GroupServiceAccountMembershipResourceModel{
		ID:               types.StringValue(membership.GroupID + "/" + membership.ServiceAccountID),
		GroupID:          types.StringValue(membership.GroupID),
		ServiceAccountID: types.StringValue(membership.ServiceAccountID),
	}
}
