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
	_ resource.Resource                = &GroupResource{}
	_ resource.ResourceWithConfigure   = &GroupResource{}
	_ resource.ResourceWithImportState = &GroupResource{}
)

func NewGroupResource() resource.Resource {
	return &GroupResource{}
}

type GroupResource struct {
	client api.Client
}

type GroupModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

func (r *GroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (r *GroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages RWX group metadata. Permissions, memberships, and relationships are managed separately and are not changed by importing or updating this resource. Renaming a group preserves its ID. RWX rejects deletion of protected groups, including system groups and groups whose deletion would remove required access.\n\n## Required permissions\n\nThe provider's access token needs `org:access` to read groups, `group:create` to create them, and `group:manage` on the target group to update or delete it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "The stable ID of the group. Use this ID to import an existing group.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Description: "The name of the group.",
				Required:    true,
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"description": schema.StringAttribute{
				Description: "The description of the group. Omit to clear the description.",
				Optional:    true,
			},
		},
	}
}

func (r *GroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan GroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	group, err := r.client.CreateGroup(api.Group{Name: plan.Name.ValueString(), Description: plan.Description.ValueStringPointer()})
	if err != nil {
		resp.Diagnostics.AddError("Error creating group in RWX", err.Error())
		return
	}
	state := groupModel(group)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *GroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state GroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	group, err := r.client.GetGroup(state.ID.ValueString())
	if errors.Is(err, api.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading group from RWX", err.Error())
		return
	}
	state = groupModel(group)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *GroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state GroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	group, err := r.client.UpdateGroup(api.Group{ID: state.ID.ValueString(), Name: plan.Name.ValueString(), Description: plan.Description.ValueStringPointer()})
	if err != nil {
		resp.Diagnostics.AddError("Error updating group in RWX", err.Error())
		return
	}
	state = groupModel(group)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *GroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state GroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteGroup(state.ID.ValueString()); err != nil && !errors.Is(err, api.ErrNotFound) {
		resp.Diagnostics.AddError("Error deleting group from RWX", "RWX rejected the group deletion: "+err.Error())
	}
}

func (r *GroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func groupModel(group api.Group) GroupModel {
	return GroupModel{
		ID:          types.StringValue(group.ID),
		Name:        types.StringValue(group.Name),
		Description: types.StringPointerValue(group.Description),
	}
}
