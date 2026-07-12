package provider

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	tfpath "github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure that the resource satisfies various framework interfaces.
var (
	_ resource.Resource                = &VariableResource{}
	_ resource.ResourceWithConfigure   = &VariableResource{}
	_ resource.ResourceWithImportState = &VariableResource{}
)

func NewVariableResource() resource.Resource {
	return &VariableResource{}
}

type VariableResource struct {
	client api.Client
}

// VariableResourceModel describes the resource data model.
type VariableResourceModel struct {
	Vault types.String `tfsdk:"vault"`
	Name  types.String `tfsdk:"name"`
	Value types.String `tfsdk:"value"`
}

func (r *VariableResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_variable"
}

func (r *VariableResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a non-secret variable stored in an RWX vault.",
		Attributes: map[string]schema.Attribute{
			"vault": schema.StringAttribute{
				Description: "The name of a vault in RWX that should hold this variable.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[a-zA-Z0-9_-]*$`),
						"can only include alphanumeric characters, dashes, or underscores",
					),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the variable itself.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[a-zA-Z0-9_-]*$`),
						"can only include alphanumeric characters, dashes, or underscores",
					),
				},
			},
			"value": schema.StringAttribute{
				Description: "The value of this variable.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
		},
	}
}

func (r *VariableResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
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

func (r *VariableResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var err error
	var plan VariableResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vault := plan.Vault.ValueString()
	variable := api.Variable{
		Name:  plan.Name.ValueString(),
		Value: plan.Value.ValueString(),
	}

	// RWX's backend only supports upserts to the variables. As a result, this 'create' operation
	// could overwrite existing variables - we protect against this by explicitly checking for the
	// existence of a variable beforehand.
	_, err = r.client.GetVariableInVault(vault, variable)
	if err == nil {
		resp.Diagnostics.AddError(
			"Variable already exists in Vault - please choose a different name or vault",
			fmt.Sprintf("Vault %q already contains a variable with name %q", vault, variable.Name),
		)
		return
	} else if !errors.Is(err, api.ErrNotFound) {
		resp.Diagnostics.AddError(
			"Error creating variable in RWX",
			"Unexpected error: "+err.Error(),
		)
		return
	}

	_, err = r.client.SetVariableInVault(vault, variable)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating variable in RWX",
			"Unexpected error: "+err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *VariableResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var err error
	var state VariableResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vault := state.Vault.ValueString()
	variable := api.Variable{
		Name: state.Name.ValueString(),
	}

	variable, err = r.client.GetVariableInVault(vault, variable)
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError(
			"Error reading variable from RWX",
			"Unexpected error: "+err.Error(),
		)
		return
	}

	state.Value = types.StringValue(variable.Value)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *VariableResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var err error
	var plan VariableResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vault := plan.Vault.ValueString()
	variable := api.Variable{
		Name:  plan.Name.ValueString(),
		Value: plan.Value.ValueString(),
	}

	_, err = r.client.SetVariableInVault(vault, variable)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating variable in RWX",
			"Unexpected error: "+err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *VariableResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var err error
	var state VariableResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vault := state.Vault.ValueString()
	variable := api.Variable{
		Name: state.Name.ValueString(),
	}

	if err = r.client.DeleteVariableInVault(vault, variable); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting variable in RWX",
			"Unexpected error: "+err.Error(),
		)
		return
	}
}

func (r *VariableResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	vault, name := path.Split(req.ID)

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, tfpath.Root("vault"), path.Clean(vault))...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, tfpath.Root("name"), name)...)
}
