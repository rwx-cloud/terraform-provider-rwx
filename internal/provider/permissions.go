package provider

import (
	"context"
	"fmt"
	"regexp"
	"slices"

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type PermissionModel struct {
	Key        types.String `tfsdk:"key"`
	ResourceID types.String `tfsdk:"resource_id"`
}

func permissionsAttribute() schema.SetNestedAttribute {
	return schema.SetNestedAttribute{
		Description: "The complete direct-permission set. Use an empty set to clear direct assignments. Inherited permissions are unchanged.",
		Required:    true,
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"key": schema.StringAttribute{
				MarkdownDescription: "The unscoped permission key, such as `run:read` or `vault:manage`.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`\S`), "must not be blank")},
			},
			"resource_id": schema.StringAttribute{
				Description: "The resource ID for a scoped assignment. Omit for organization-wide access.",
				Optional:    true,
				Validators:  []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`\S`), "must not be blank")},
			},
		}},
	}
}

func permissionsFromModel(ctx context.Context, value types.Set) ([]api.Permission, diag.Diagnostics) {
	var models []PermissionModel
	diags := value.ElementsAs(ctx, &models, false)
	permissions := make([]api.Permission, 0, len(models))
	for _, model := range models {
		permissions = append(permissions, api.Permission{Key: model.Key.ValueString(), ResourceID: model.ResourceID.ValueStringPointer()})
	}
	return permissions, diags
}

func permissionsModel(ctx context.Context, permissions []api.Permission) (types.Set, diag.Diagnostics) {
	models := make([]PermissionModel, 0, len(permissions))
	for _, permission := range permissions {
		models = append(models, PermissionModel{Key: types.StringValue(permission.Key), ResourceID: types.StringPointerValue(permission.ResourceID)})
	}
	return types.SetValueFrom(ctx, types.ObjectType{AttrTypes: map[string]attr.Type{"key": types.StringType, "resource_id": types.StringType}}, models)
}

func canonicalPermission(key string, resourceID *string) string {
	if resourceID != nil {
		return fmt.Sprintf("%s(id: %s)", key, *resourceID)
	}
	return key
}

func validatePermissions(ctx context.Context, planned types.Set, catalog api.PermissionCatalog, current []api.Permission, targetKnown bool) diag.Diagnostics {
	var diags diag.Diagnostics
	if planned.IsUnknown() || planned.IsNull() {
		return diags
	}
	known := map[string]api.CatalogPermission{}
	for _, permission := range catalog.Permissions {
		known[permission.Key] = permission
	}
	existing := map[string]bool{}
	for _, permission := range current {
		existing[canonicalPermission(permission.Key, permission.ResourceID)] = true
	}
	desired := map[string]bool{}
	hasUnknown := false
	for _, element := range planned.Elements() {
		if element.IsUnknown() {
			hasUnknown = true
			continue
		}
		attributes := element.(types.Object).Attributes()
		key := attributes["key"].(types.String)
		scope := attributes["resource_id"].(types.String)
		if key.IsUnknown() {
			hasUnknown = true
			continue
		}
		permission, ok := known[key.ValueString()]
		if !ok {
			diags.AddAttributeError(path.Root("permissions"), "Invalid permission key", fmt.Sprintf("Permission %q is not in the RWX permission catalog.", key.ValueString()))
			continue
		}
		if !scope.IsNull() && !permission.SupportsResourceScope {
			diags.AddAttributeError(path.Root("permissions"), "Invalid permission scope", fmt.Sprintf("Permission %q does not support resource_id.", permission.Key))
			continue
		}
		if scope.IsUnknown() {
			hasUnknown = true
			continue
		}
		canonical := canonicalPermission(permission.Key, scope.ValueStringPointer())
		desired[canonical] = true
		grantable := scope.IsNull() && permission.Grantable.Unscoped || (!scope.IsNull() && slices.Contains(permission.Grantable.ResourceIDs, scope.ValueString()))
		if targetKnown && !existing[canonical] && !grantable {
			diags.AddAttributeError(path.Root("permissions"), "Permission cannot be granted", fmt.Sprintf("The acting token cannot grant %q. Check its authority and the resource ID.", canonical))
		}
	}
	if !hasUnknown {
		for _, permission := range catalog.UnremovablePermissions {
			if !desired[permission] {
				diags.AddAttributeError(path.Root("permissions"), "Permission cannot be removed", fmt.Sprintf("The acting token cannot remove %q from this target.", permission))
			}
		}
	}
	return diags
}
