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
	_ resource.Resource                = &GroupRelationshipResource{}
	_ resource.ResourceWithConfigure   = &GroupRelationshipResource{}
	_ resource.ResourceWithImportState = &GroupRelationshipResource{}
)

func NewGroupRelationshipResource() resource.Resource {
	return &GroupRelationshipResource{}
}

type GroupRelationshipResource struct {
	client api.Client
}

type GroupRelationshipResourceModel struct {
	ID            types.String `tfsdk:"id"`
	ParentGroupID types.String `tfsdk:"parent_group_id"`
	ChildGroupID  types.String `tfsdk:"child_group_id"`
}

func (r *GroupRelationshipResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_relationship"
}

func (r *GroupRelationshipResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one direct parent/child group relationship. Children inherit their parents' permissions; parents do not inherit their children's permissions. A group can have multiple parents and children. Other relationships are preserved. Requires `group:manage` on the parent to add or remove the relationship. The server rejects cycles and removals that would leave the organization without an administrator.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The relationship identity, in parent-group-id/child-group-id format.",
				Computed:    true,
			},
			"parent_group_id": schema.StringAttribute{
				Description:   "The ID of the parent group whose permissions the child inherits. Changing this replaces the relationship.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"child_group_id": schema.StringAttribute{
				Description:   "The ID of the child group that inherits the parent's permissions. Changing this replaces the relationship.",
				Required:      true,
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
		},
	}
}

func (r *GroupRelationshipResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GroupRelationshipResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan GroupRelationshipResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	relationship, err := r.client.CreateGroupRelationship(plan.ParentGroupID.ValueString(), plan.ChildGroupID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating group relationship in RWX", err.Error())
		return
	}
	state := groupRelationshipResourceModel(relationship)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *GroupRelationshipResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state GroupRelationshipResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	relationship, err := r.client.GetGroupRelationship(state.ParentGroupID.ValueString(), state.ChildGroupID.ValueString())
	if errors.Is(err, api.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading group relationship from RWX", err.Error())
		return
	}
	state = groupRelationshipResourceModel(relationship)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *GroupRelationshipResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Unexpected group relationship update", "Changing either group ID requires replacing the relationship.")
}

func (r *GroupRelationshipResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state GroupRelationshipResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteGroupRelationship(state.ParentGroupID.ValueString(), state.ChildGroupID.ValueString())
	if err != nil && !errors.Is(err, api.ErrNotFound) {
		resp.Diagnostics.AddError("Error deleting group relationship from RWX", err.Error())
	}
}

func (r *GroupRelationshipResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid group relationship import ID", "Use parent-group-id/child-group-id.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("parent_group_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("child_group_id"), parts[1])...)
}

func groupRelationshipResourceModel(relationship api.GroupRelationship) GroupRelationshipResourceModel {
	return GroupRelationshipResourceModel{
		ID:            types.StringValue(relationship.ParentGroupID + "/" + relationship.ChildGroupID),
		ParentGroupID: types.StringValue(relationship.ParentGroupID),
		ChildGroupID:  types.StringValue(relationship.ChildGroupID),
	}
}
