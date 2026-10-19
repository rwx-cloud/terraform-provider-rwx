package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	_ resource.Resource                = &GroupUserMembershipResource{}
	_ resource.ResourceWithConfigure   = &GroupUserMembershipResource{}
	_ resource.ResourceWithImportState = &GroupUserMembershipResource{}
)

func NewGroupUserMembershipResource() resource.Resource {
	return &GroupUserMembershipResource{}
}

type GroupUserMembershipResource struct {
	client api.Client
}

type GroupUserMembershipResourceModel struct {
	ID      types.String `tfsdk:"id"`
	GroupID types.String `tfsdk:"group_id"`
	UserID  types.String `tfsdk:"user_id"`
}

func (r *GroupUserMembershipResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_user_membership"
}

func (r *GroupUserMembershipResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one direct manual user membership in a group. Other users, inherited membership, and directory-owned membership are preserved. Import reads only the manual contribution; inherited or directory-only membership cannot be imported. Manual user membership changes are rejected in SCIM-managed organizations. Requires `org:access` to read and `group:manage` on the group to add or remove membership.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The membership identity, in group-id/user-id format.",
				Computed:    true,
			},
			"group_id": schema.StringAttribute{
				Description:   "The ID of the group. Changing this replaces the membership.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"user_id": schema.StringAttribute{
				Description:   "The user ID of an existing active user in the organization, not an identity ID or organization-user ID. Changing this replaces the membership.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
		},
	}
}

func (r *GroupUserMembershipResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GroupUserMembershipResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan GroupUserMembershipResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	membership, err := r.client.CreateGroupUserMembership(plan.GroupID.ValueString(), plan.UserID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating group user membership in RWX", err.Error())
		return
	}
	state := groupUserMembershipResourceModel(membership)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *GroupUserMembershipResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state GroupUserMembershipResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	membership, err := r.client.GetGroupUserMembership(state.GroupID.ValueString(), state.UserID.ValueString())
	if errors.Is(err, api.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading group user membership from RWX", err.Error())
		return
	}
	state = groupUserMembershipResourceModel(membership)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *GroupUserMembershipResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Unexpected group user membership update", "Changing either ID requires replacing the membership.")
}

func (r *GroupUserMembershipResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state GroupUserMembershipResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteGroupUserMembership(state.GroupID.ValueString(), state.UserID.ValueString())
	if err != nil && !errors.Is(err, api.ErrNotFound) {
		resp.Diagnostics.AddError("Error deleting group user membership from RWX", err.Error())
	}
}

func (r *GroupUserMembershipResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid group user membership import ID", "Use group-id/user-id.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), parts[1])...)
}

func groupUserMembershipResourceModel(membership api.GroupUserMembership) GroupUserMembershipResourceModel {
	return GroupUserMembershipResourceModel{
		ID:      types.StringValue(membership.GroupID + "/" + membership.UserID),
		GroupID: types.StringValue(membership.GroupID),
		UserID:  types.StringValue(membership.UserID),
	}
}
