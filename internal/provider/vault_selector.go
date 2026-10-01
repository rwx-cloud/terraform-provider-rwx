package provider

import (
	"encoding/json"
	"errors"
	"regexp"

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type vaultSelectorModel struct {
	vault   types.String
	vaultID types.String
}

func vaultNameAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Description: description,
		Optional:    true,
		Validators: []validator.String{
			stringvalidator.LengthAtLeast(1),
			stringvalidator.RegexMatches(
				regexp.MustCompile(`^[a-zA-Z0-9_-]*$`),
				"can only include alphanumeric characters, dashes, or underscores",
			),
		},
	}
}

func vaultIDAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Description: description,
		Optional:    true,
		Validators: []validator.String{
			stringvalidator.LengthAtLeast(1),
			stringvalidator.ExactlyOneOf(path.MatchRoot("vault")),
		},
	}
}

func configuredVaultSelector(model vaultSelectorModel) api.VaultSelector {
	if !model.vaultID.IsNull() && !model.vaultID.IsUnknown() {
		return api.VaultSelector{ID: model.vaultID.ValueString()}
	}
	if !model.vault.IsNull() && !model.vault.IsUnknown() {
		return api.VaultSelector{Name: model.vault.ValueString()}
	}

	return api.VaultSelector{}
}

func resolveVaultID(client api.Client, model vaultSelectorModel, previousID string) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	selector := configuredVaultSelector(model)
	if selector.ID != "" {
		return selector.ID, diags
	}
	if previousID != "" {
		return previousID, diags
	}
	if selector.Name == "" {
		return "", diags
	}

	vault, err := client.FindVaultByName(selector.Name)
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			diags.AddError("Vault not found", "RWX does not contain a vault named "+selector.Name+".")
		} else {
			diags.AddError("Error resolving vault in RWX", "Unexpected error: "+err.Error())
		}
		return "", diags
	}

	return vault.ID, diags
}

func vaultSelectorsRequireReplace(client api.Client, state vaultSelectorModel, plan vaultSelectorModel, previousID string) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	if state.vault.Equal(plan.vault) && state.vaultID.Equal(plan.vaultID) {
		return false, diags
	}
	if plan.vault.IsUnknown() || plan.vaultID.IsUnknown() {
		return true, diags
	}

	stateID, stateDiags := resolveVaultID(client, state, previousID)
	diags.Append(stateDiags...)
	planID, planDiags := resolveVaultID(client, plan, "")
	diags.Append(planDiags...)
	if diags.HasError() {
		return false, diags
	}

	return stateID != planID, diags
}

func decodePrivateVaultID(value []byte) (string, error) {
	if len(value) == 0 {
		return "", nil
	}

	var vaultID string
	if err := json.Unmarshal(value, &vaultID); err != nil {
		return "", err
	}

	return vaultID, nil
}

func encodePrivateVaultID(vaultID string) ([]byte, error) {
	return json.Marshal(vaultID)
}

func vaultReplacementPath(model vaultSelectorModel) path.Path {
	if !model.vaultID.IsNull() {
		return path.Root("vault_id")
	}

	return path.Root("vault")
}

func privateVaultID(value []byte, diags *diag.Diagnostics) string {
	vaultID, err := decodePrivateVaultID(value)
	if err != nil {
		diags.AddError("Error reading vault state", "Unexpected error: "+err.Error())
	}

	return vaultID
}
