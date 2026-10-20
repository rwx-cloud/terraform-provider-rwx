package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = &DirectoryGroupMappingResource{}
	_ resource.ResourceWithConfigure      = &DirectoryGroupMappingResource{}
	_ resource.ResourceWithImportState    = &DirectoryGroupMappingResource{}
	_ resource.ResourceWithModifyPlan     = &DirectoryGroupMappingResource{}
	_ resource.ResourceWithValidateConfig = &DirectoryGroupMappingResource{}
)

func NewDirectoryGroupMappingResource() resource.Resource {
	return &DirectoryGroupMappingResource{}
}

type DirectoryGroupMappingResource struct {
	client api.Client
}

type DirectoryGroupMappingResourceModel struct {
	ID                    types.String `tfsdk:"id"`
	DirectoryID           types.String `tfsdk:"directory_id"`
	DirectoryGroupID      types.String `tfsdk:"directory_group_id"`
	GroupID               types.String `tfsdk:"group_id"`
	ReconciliationStatus  types.String `tfsdk:"reconciliation_status"`
	ReconciliationTimeout types.String `tfsdk:"reconciliation_timeout"`
}

func (r *DirectoryGroupMappingResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_directory_group_mapping"
}

func (r *DirectoryGroupMappingResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Maps one imported directory group to an RWX group. Multiple directory groups may contribute to one target; removing or retargeting one source preserves memberships contributed by the others. Import using the local directory-group ID, not its external IdP ID.\n\n## Setup\n\nConnect and import the directory, review memberships, and activate directory sync in the UI. This resource does not manage activation or IdP rosters. See [directory sync](https://www.rwx.com/docs/security/directory-sync).\n\n## Reconciliation and retries\n\nDraft saves finish immediately. Once sync is enabled, creates, retargets, and removals wait for membership reconciliation, including when enforcement is subsequently disabled. `reconciliation_timeout` controls the wait per write and defaults to `20m`.\n\nIf a save fails or times out, run `terraform apply` again to retry the desired target. Terraform may mark a failed create as tainted and replace it on the next apply. If a destroy fails or times out, run `terraform destroy` again. The provider reports an error unless RWX confirms reconciliation succeeded; check the error before retrying if another operation changed the mapping.\n\n## Required permissions\n\nThe provider's access token needs `group:manage` on all affected targets, including former targets. If the provider no longer has the target information needed to remove a mapping, removal requires unscoped `group:manage`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The stable mapping ID (the local directory-group ID).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"directory_id": schema.StringAttribute{
				Description: "The local directory ID. Changing the source replaces the mapping.",
				Required:    true,
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"directory_group_id": schema.StringAttribute{
				Description: "The local directory-group ID. Changing the source replaces the mapping.",
				Required:    true,
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"group_id": schema.StringAttribute{
				Description: "The target RWX group ID. Changes retarget this source in place.",
				Required:    true,
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"reconciliation_status": schema.StringAttribute{
				Description: "The current membership reconciliation status. An incomplete or failed save is retried by the next apply.",
				Computed:    true,
			},
			"reconciliation_timeout": schema.StringAttribute{
				Description: "Maximum wait for membership reconciliation per write, as a positive Go duration (for example 20m or 90s). Default: 20m.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("20m"),
			},
		},
	}
}

func (r *DirectoryGroupMappingResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DirectoryGroupMappingResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var timeout types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("reconciliation_timeout"), &timeout)...)
	if !resp.Diagnostics.HasError() {
		if _, err := mappingTimeout(timeout); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("reconciliation_timeout"), "Invalid reconciliation timeout", err.Error())
		}
	}
}

func (r *DirectoryGroupMappingResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var state DirectoryGroupMappingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if !resp.Diagnostics.HasError() && state.ReconciliationStatus.ValueString() != "succeeded" {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("reconciliation_status"), types.StringUnknown())...)
	}
}

func (r *DirectoryGroupMappingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DirectoryGroupMappingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := r.save(ctx, &plan, true)
	if result.Mapping != nil {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	}
	if err != nil {
		resp.Diagnostics.AddError("Error saving directory group mapping in RWX", err.Error())
	}
}

func (r *DirectoryGroupMappingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DirectoryGroupMappingResourceModel
	var state DirectoryGroupMappingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = state.ID
	removal, diags := req.Private.GetKey(ctx, "removal")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := r.save(ctx, &plan, len(removal) != 0)
	if result.Mapping != nil {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, "removal", nil)...)
	}
	if err != nil {
		resp.Diagnostics.AddError("Error saving directory group mapping in RWX", err.Error())
	}
}

func (r *DirectoryGroupMappingResource) save(ctx context.Context, plan *DirectoryGroupMappingResourceModel, create bool) (api.DirectoryGroupMappingResponse, error) {
	timeout, err := mappingTimeout(plan.ReconciliationTimeout)
	if err != nil {
		return api.DirectoryGroupMappingResponse{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var result api.DirectoryGroupMappingResponse
	if create {
		result, err = r.client.CreateDirectoryGroupMapping(ctx, api.DirectoryGroupMapping{DirectoryID: plan.DirectoryID.ValueString(), DirectoryGroupID: plan.DirectoryGroupID.ValueString(), GroupID: plan.GroupID.ValueString()})
	} else {
		result, err = r.client.UpdateDirectoryGroupMapping(ctx, plan.ID.ValueString(), plan.GroupID.ValueString())
	}
	if err != nil {
		if saved, readErr := r.client.GetDirectoryGroupMapping(ctx, plan.DirectoryGroupID.ValueString()); readErr == nil && saved.Mapping != nil && saved.Mapping.DirectoryID == plan.DirectoryID.ValueString() && saved.Mapping.GroupID == plan.GroupID.ValueString() {
			result = saved
			plan.setMapping(result)
		}
		return result, err
	}
	if result.Mapping == nil {
		return result, fmt.Errorf("RWX returned no saved mapping")
	}
	plan.setMapping(result)
	completion, err := r.wait(ctx, result.Mapping.ID, result.Reconciliation)
	plan.ReconciliationStatus = types.StringValue(completion.Status)
	return result, err
}

func (m *DirectoryGroupMappingResourceModel) setMapping(result api.DirectoryGroupMappingResponse) {
	m.ID = types.StringValue(result.Mapping.ID)
	m.DirectoryID = types.StringValue(result.Mapping.DirectoryID)
	m.DirectoryGroupID = types.StringValue(result.Mapping.DirectoryGroupID)
	m.GroupID = types.StringValue(result.Mapping.GroupID)
	m.ReconciliationStatus = types.StringValue(result.Reconciliation.Status)
}

func (r *DirectoryGroupMappingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DirectoryGroupMappingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.ReconciliationTimeout.IsNull() {
		state.ReconciliationTimeout = types.StringValue("20m")
	}
	result, err := r.client.GetDirectoryGroupMapping(ctx, state.ID.ValueString())
	if errors.Is(err, api.ErrNotFound) {
		removal, diags := req.Private.GetKey(ctx, "removal")
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if len(removal) == 0 {
			resp.State.RemoveResource(ctx)
			return
		}
		var previous api.DirectoryGroupMappingReconciliation
		if err := json.Unmarshal(removal, &previous); err != nil {
			resp.Diagnostics.AddError("Invalid mapping removal state", err.Error())
			return
		}
		completion, pollErr := r.client.GetDirectoryGroupMappingReconciliation(ctx, state.ID.ValueString(), previous.ID)
		if pollErr != nil {
			resp.Diagnostics.AddError("Error reading mapping removal from RWX", pollErr.Error())
			return
		}
		if completion.Status == "succeeded" {
			resp.State.RemoveResource(ctx)
			return
		}
		state.ReconciliationStatus = types.StringValue(completion.Status)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading directory group mapping from RWX", err.Error())
		return
	}
	if result.Mapping == nil {
		resp.Diagnostics.AddError("Error reading directory group mapping from RWX", "RWX returned no mapping.")
		return
	}
	state.setMapping(result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DirectoryGroupMappingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DirectoryGroupMappingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, err := mappingTimeout(state.ReconciliationTimeout)
	if err != nil {
		resp.Diagnostics.AddError("Invalid reconciliation timeout", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := r.client.DeleteDirectoryGroupMapping(ctx, state.ID.ValueString())
	if errors.Is(err, api.ErrNotFound) {
		return
	}
	if err != nil {
		completion, pollErr := r.client.GetDirectoryGroupMappingReconciliation(ctx, state.ID.ValueString(), "")
		if pollErr == nil && completion.ID != "" {
			encoded, _ := json.Marshal(completion)
			resp.Diagnostics.Append(resp.Private.SetKey(ctx, "removal", encoded)...)
		}
	}
	if err == nil {
		if result.Reconciliation.ID != "" {
			encoded, _ := json.Marshal(result.Reconciliation)
			resp.Diagnostics.Append(resp.Private.SetKey(ctx, "removal", encoded)...)
		}
		_, err = r.wait(ctx, state.ID.ValueString(), result.Reconciliation)
	}
	if err != nil {
		resp.Diagnostics.AddError("Error removing directory group mapping from RWX", err.Error())
	}
}

func (r *DirectoryGroupMappingResource) wait(ctx context.Context, id string, completion api.DirectoryGroupMappingReconciliation) (api.DirectoryGroupMappingReconciliation, error) {
	for {
		switch completion.Status {
		case "succeeded":
			return completion, nil
		case "pending":
			if completion.ID == "" {
				return completion, fmt.Errorf("pending reconciliation has no ID; retry apply")
			}
		case "failed", "unknown":
			return completion, fmt.Errorf("membership reconciliation %s: %s; retry apply", completion.Status, completion.Error)
		default:
			return completion, fmt.Errorf("unexpected reconciliation status %q; retry apply", completion.Status)
		}
		current, err := r.client.GetDirectoryGroupMappingReconciliation(ctx, id, completion.ID)
		if err != nil {
			return completion, err
		}
		completion = current
		if completion.Status == "pending" {
			select {
			case <-ctx.Done():
				return completion, fmt.Errorf("waiting for membership reconciliation: %w; retry apply", ctx.Err())
			case <-time.After(time.Second):
			}
		}
	}
}

func mappingTimeout(value types.String) (time.Duration, error) {
	if value.IsNull() || value.IsUnknown() {
		return 20 * time.Minute, nil
	}
	timeout, err := time.ParseDuration(value.ValueString())
	if err != nil || timeout <= 0 {
		return 0, fmt.Errorf("reconciliation_timeout must be a positive duration, got %q", value.ValueString())
	}
	return timeout, nil
}

func (r *DirectoryGroupMappingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
