package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure that the resource satisfies various framework interfaces.
var (
	_ resource.Resource              = &SecretResource{}
	_ resource.ResourceWithConfigure = &SecretResource{}
)

func NewSecretResource() resource.Resource {
	return &SecretResource{}
}

type SecretResource struct {
	client api.Client
}

// SecretResourceModel describes the resource data model.
type SecretResourceModel struct {
	Vault       types.String `tfsdk:"vault"`
	Name        types.String `tfsdk:"name"`
	SecretValue types.String `tfsdk:"secret_value"`
	Description types.String `tfsdk:"description"`
}

func (r *SecretResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret"
}

func (r *SecretResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a secret stored in an RWX vault.",
		Attributes: map[string]schema.Attribute{
			"vault": schema.StringAttribute{
				Description: "The name of a vault in RWX that should hold this secret.",
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
				Description: "The name of the secret itself.",
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
			"secret_value": schema.StringAttribute{
				Description: "The secret value.",
				Required:    true,
				Sensitive:   true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": schema.StringAttribute{
				Description: "An optional description of this secret.",
				Optional:    true,
			},
		},
	}
}

func (r *SecretResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var err error
	var plan SecretResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vault := plan.Vault.ValueString()
	secret := api.Secret{
		Name:        plan.Name.ValueString(),
		SecretValue: plan.SecretValue.ValueString(),
		Description: plan.Description.ValueString(),
	}

	// RWX's backend only supports upserts to the secrets. As a result, this 'create' operation
	// could overwrite existing secrets - we protect against this by explicitly checking for the
	// existence of a secret beforehand.
	_, err = r.client.GetSecretMetadataInVault(vault, secret)
	if err == nil {
		resp.Diagnostics.AddError(
			"Secret already exists in Vault - please choose a different name or vault",
			fmt.Sprintf("Vault %q already contains a secret with name %q", vault, secret.Name),
		)
		return
	} else if !errors.Is(err, api.ErrNotFound) {
		resp.Diagnostics.AddError(
			"Error creating secret in RWX",
			"Unexpected error: "+err.Error(),
		)
		return
	}

	secret, err = r.client.SetSecretInVault(vault, secret)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating secret in RWX",
			"Unexpected error: "+err.Error(),
		)
		return
	}

	resp.Private.SetKey(ctx, "version", []byte(strconv.Itoa(secret.Version)))

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *SecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var err error
	var state SecretResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vault := state.Vault.ValueString()
	secret := api.Secret{
		Name: state.Name.ValueString(),
	}

	secret, err = r.client.GetSecretMetadataInVault(vault, secret)
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError(
			"Error reading secret metadata from RWX",
			"Unexpected error: "+err.Error(),
		)
		return
	}

	if secret.Description != "" {
		state.Description = types.StringValue(secret.Description)
	}

	version, diags := req.Private.GetKey(ctx, "version")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if strconv.Itoa(secret.Version) != string(version) {
		state.SecretValue = types.StringValue("")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var err error
	var plan SecretResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vault := plan.Vault.ValueString()
	secret := api.Secret{
		Name:        plan.Name.ValueString(),
		SecretValue: plan.SecretValue.ValueString(),
		Description: plan.Description.ValueString(),
	}

	secret, err = r.client.SetSecretInVault(vault, secret)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating secret in RWX",
			"Unexpected error: "+err.Error(),
		)
		return
	}

	resp.Private.SetKey(ctx, "version", []byte(strconv.Itoa(secret.Version)))

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *SecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var err error
	var state SecretResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vault := state.Vault.ValueString()
	secret := api.Secret{
		Name: state.Name.ValueString(),
	}

	if err = r.client.DeleteSecretInVault(vault, secret); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting secret in RWX",
			"Unexpected error: "+err.Error(),
		)
		return
	}
}
