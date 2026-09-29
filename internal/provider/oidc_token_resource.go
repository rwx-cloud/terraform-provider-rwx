package provider

import (
	"context"
	"encoding/json"
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
	_ resource.Resource                = &OIDCTokenResource{}
	_ resource.ResourceWithConfigure   = &OIDCTokenResource{}
	_ resource.ResourceWithImportState = &OIDCTokenResource{}
)

func NewOIDCTokenResource() resource.Resource {
	return &OIDCTokenResource{}
}

type OIDCTokenResource struct {
	client api.Client
}

type OIDCTokenResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Vault            types.String `tfsdk:"vault"`
	Name             types.String `tfsdk:"name"`
	Audience         types.String `tfsdk:"audience"`
	Subject          types.String `tfsdk:"subject"`
	Expression       types.String `tfsdk:"expression"`
	DocumentationURL types.String `tfsdk:"documentation_url"`
}

func (r *OIDCTokenResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oidc_token"
}

func (r *OIDCTokenResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an OIDC token definition stored in an RWX vault.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the OIDC token definition.",
				Computed:    true,
			},
			"vault": schema.StringAttribute{
				Description: "The name of the RWX vault that holds the OIDC token definition.",
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
				Description: "The name of the OIDC token definition.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[a-zA-Z0-9_-]*$`),
						"can only include alphanumeric characters, dashes, or underscores",
					),
				},
			},
			"audience": schema.StringAttribute{
				Description: "The audience claim included in tokens issued from this definition.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"subject": schema.StringAttribute{
				Description: "The subject claim included in issued tokens.",
				Computed:    true,
			},
			"expression": schema.StringAttribute{
				Description: "The expression used to reference the OIDC token in an RWX run.",
				Computed:    true,
			},
			"documentation_url": schema.StringAttribute{
				Description: "The RWX documentation URL for this OIDC token definition.",
				Computed:    true,
			},
		},
	}
}

func (r *OIDCTokenResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OIDCTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan OIDCTokenResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token, err := r.client.CreateOIDCToken(plan.Vault.ValueString(), api.OIDCToken{
		Name:     plan.Name.ValueString(),
		Audience: plan.Audience.ValueString(),
	})
	if err != nil {
		if errors.Is(err, api.ErrConflict) {
			resp.Diagnostics.AddError(
				"OIDC token already exists",
				fmt.Sprintf("Vault %q already contains an OIDC token named %q. Import the existing token by its ID or choose a different name.", plan.Vault.ValueString(), plan.Name.ValueString()),
			)
			return
		}

		resp.Diagnostics.AddError("Error creating OIDC token in RWX", "Unexpected error: "+err.Error())
		return
	}

	state := oidcTokenResourceModel(token)
	vaultID, err := json.Marshal(token.Vault.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error storing OIDC token state", "Unexpected error: "+err.Error())
		return
	}
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, "vault_id", vaultID)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *OIDCTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state OIDCTokenResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vaultIDJSON, diags := req.Private.GetKey(ctx, "vault_id")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var token api.OIDCToken
	var err error
	if len(vaultIDJSON) == 0 {
		token, err = r.client.FindOIDCToken(state.ID.ValueString())
	} else {
		var vaultID string
		if err := json.Unmarshal(vaultIDJSON, &vaultID); err != nil {
			resp.Diagnostics.AddError("Error reading OIDC token state", "Unexpected error: "+err.Error())
			return
		}
		token, err = r.client.GetOIDCToken(vaultID, state.ID.ValueString())
	}
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError("Error reading OIDC token from RWX", "Unexpected error: "+err.Error())
		return
	}

	state = oidcTokenResourceModel(token)
	vaultID, err := json.Marshal(token.Vault.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error storing OIDC token state", "Unexpected error: "+err.Error())
		return
	}
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, "vault_id", vaultID)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *OIDCTokenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan OIDCTokenResourceModel
	var state OIDCTokenResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vaultIDJSON, diags := req.Private.GetKey(ctx, "vault_id")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	var vaultID string
	if err := json.Unmarshal(vaultIDJSON, &vaultID); err != nil {
		resp.Diagnostics.AddError("Error reading OIDC token state", "Unexpected error: "+err.Error())
		return
	}

	token, err := r.client.UpdateOIDCToken(vaultID, api.OIDCToken{
		ID:       state.ID.ValueString(),
		Name:     plan.Name.ValueString(),
		Audience: plan.Audience.ValueString(),
	})
	if err != nil {
		if errors.Is(err, api.ErrConflict) {
			resp.Diagnostics.AddError(
				"OIDC token name is already in use",
				fmt.Sprintf("Vault %q already contains another OIDC token named %q.", plan.Vault.ValueString(), plan.Name.ValueString()),
			)
			return
		}

		resp.Diagnostics.AddError("Error updating OIDC token in RWX", "Unexpected error: "+err.Error())
		return
	}

	state = oidcTokenResourceModel(token)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *OIDCTokenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state OIDCTokenResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vaultIDJSON, diags := req.Private.GetKey(ctx, "vault_id")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	var vaultID string
	if err := json.Unmarshal(vaultIDJSON, &vaultID); err != nil {
		resp.Diagnostics.AddError("Error reading OIDC token state", "Unexpected error: "+err.Error())
		return
	}

	err := r.client.DeleteOIDCToken(vaultID, state.ID.ValueString())
	if err != nil && !errors.Is(err, api.ErrNotFound) {
		resp.Diagnostics.AddError("Error deleting OIDC token from RWX", "Unexpected error: "+err.Error())
	}
}

func (r *OIDCTokenResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func oidcTokenResourceModel(token api.OIDCToken) OIDCTokenResourceModel {
	return OIDCTokenResourceModel{
		ID:               types.StringValue(token.ID),
		Vault:            types.StringValue(token.Vault.Name),
		Name:             types.StringValue(token.Name),
		Audience:         types.StringValue(token.Audience),
		Subject:          types.StringValue(token.Subject),
		Expression:       types.StringValue(token.Expression),
		DocumentationURL: types.StringValue(token.DocumentationURL),
	}
}
