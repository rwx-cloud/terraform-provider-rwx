package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"
)

func TestIntegrationIdentityLifecycle(t *testing.T) {
	client := integrationClient(t)
	group := integrationGroup(t, client)
	account := integrationAccount(t, client)
	initialGroupName, immutableName := group.Name, account.Name
	config := func(name, description string) string {
		return providerConfig + fmt.Sprintf(`
resource "rwx_group" "test" {
  name = %q
  %s
}
resource "rwx_service_account" "test" {
  display_name = %q
  %s
}
data "rwx_group" "test" {
  name = rwx_group.test.name
  depends_on = [rwx_group.test]
}
data "rwx_service_account" "test" {
  key = rwx_service_account.test.key
}
`, name, description, name, description)
	}
	check := func(name string) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("rwx_group.test", "id", group.ID),
			resource.TestCheckResourceAttr("data.rwx_group.test", "id", group.ID),
			resource.TestCheckResourceAttr("rwx_service_account.test", "id", account.ID),
			resource.TestCheckResourceAttr("data.rwx_service_account.test", "id", account.ID),
			resource.TestCheckResourceAttr("rwx_service_account.test", "key", immutableName),
			resource.TestCheckResourceAttr("rwx_group.test", "name", name),
			resource.TestCheckResourceAttr("rwx_service_account.test", "display_name", name),
		)
	}
	initial := config(initialGroupName, `description = "integration fixture"`)
	updated := config(initialGroupName+"-updated", "")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: initial + fmt.Sprintf(`
import {
  to = rwx_group.test
  id = %q
}
import {
  to = rwx_service_account.test
  id = %q
}
`, group.ID, account.ID), Check: check(initialGroupName)},
			{Config: initial, PlanOnly: true},
			{Config: updated, Check: resource.ComposeAggregateTestCheckFunc(
				check(initialGroupName+"-updated"),
				resource.TestCheckNoResourceAttr("rwx_group.test", "description"),
				resource.TestCheckNoResourceAttr("rwx_service_account.test", "description"),
			)},
			{Config: updated, PlanOnly: true},
		},
	})
	_, err := client.GetGroup(group.ID)
	integrationError(t, err, api.ErrNotFound)
	_, err = client.FindGroupByName(initialGroupName + "-updated")
	integrationError(t, err, api.ErrNotFound)
	_, err = client.GetServiceAccount(account.ID)
	integrationError(t, err, api.ErrNotFound)
	_, err = client.FindServiceAccount(immutableName)
	integrationError(t, err, api.ErrNotFound)
}

func TestIntegrationServiceAccountToken(t *testing.T) {
	client := integrationClient(t)
	name := integrationName()
	config := func(displayName string) string {
		return providerConfig + fmt.Sprintf(`
resource "rwx_service_account" "test" {
  display_name = %q
}
output "service_account_token" {
  value = rwx_service_account.test.token
  sensitive = true
}
`, displayName)
	}
	var id, token string
	host := os.Getenv("RWX_HOST")
	if host == "" {
		host = "cloud.rwx.com"
	}
	checkAuthentication := func(wantStatus int) error {
		tokenClient, err := api.NewClient(api.Config{Host: host, AccessToken: token, Version: "test"})
		if err != nil {
			return err
		}
		req, err := http.NewRequest(http.MethodGet, "/api/auth/whoami", nil)
		if err != nil {
			return err
		}
		resp, err := tokenClient.RoundTrip(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != wantStatus {
			return fmt.Errorf("created token authentication returned %d, want %d", resp.StatusCode, wantStatus)
		}
		if wantStatus == http.StatusOK {
			var identity struct {
				OrganizationSlug string `json:"organization_slug"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&identity); err != nil {
				return err
			}
			if identity.OrganizationSlug != "rwx-integration-testing" {
				return fmt.Errorf("created token authenticated to the wrong organization")
			}
		}
		return nil
	}
	check := func(state *terraform.State) error {
		account := state.RootModule().Resources["rwx_service_account.test"].Primary
		currentToken := account.Attributes["token"]
		if currentToken == "" {
			return fmt.Errorf("Terraform did not capture the creation token")
		}
		if token != "" && currentToken != token {
			return fmt.Errorf("refresh or update changed the creation token")
		}
		id, token = account.ID, currentToken
		output := state.RootModule().Outputs["service_account_token"]
		if output == nil || !output.Sensitive || output.Value != token {
			return fmt.Errorf("Terraform output did not preserve the sensitive token reference")
		}
		shown, err := client.GetServiceAccount(id)
		if err != nil {
			return err
		}
		if shown.Token != nil {
			return fmt.Errorf("show endpoint returned a token")
		}
		listed, err := client.ListServiceAccounts(account.Attributes["key"])
		if err != nil {
			return err
		}
		if len(listed) != 1 || listed[0].ID != id || listed[0].Token != nil {
			return fmt.Errorf("list endpoint did not return token-free account metadata")
		}
		return checkAuthentication(http.StatusOK)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			if _, err := client.GetServiceAccount(id); err != api.ErrNotFound {
				return fmt.Errorf("destroy did not deactivate the service account: %v", err)
			}
			return checkAuthentication(http.StatusUnauthorized)
		},
		Steps: []resource.TestStep{
			{Config: config(name), Check: check},
			{RefreshState: true, Check: check},
			{Config: config(name + "-renamed"), Check: check},
			{ResourceName: "rwx_service_account.test", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"token"}, ImportStateCheck: func(states []*terraform.InstanceState) error {
				if states[0].Attributes["token"] != "" {
					return fmt.Errorf("import recovered a creation-only token")
				}
				return nil
			}},
			{Config: config(name + "-renamed"), PlanOnly: true, Check: check},
		},
	})
}

func TestIntegrationGroupRelationship(t *testing.T) {
	client := integrationClient(t)
	parent := integrationGroup(t, client)
	child := integrationGroup(t, client)
	other := integrationGroup(t, client)
	if _, err := client.CreateGroupRelationship(parent.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	config := providerConfig + fmt.Sprintf(`
resource "rwx_group_relationship" "test" {
  parent_group_id = %q
  child_group_id = %q
}
`, parent.ID, child.ID)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config, Check: resource.TestCheckResourceAttr("rwx_group_relationship.test", "id", parent.ID+"/"+child.ID)},
			{ResourceName: "rwx_group_relationship.test", ImportState: true, ImportStateVerify: true},
			{Config: config, PlanOnly: true},
			{PreConfig: func() {
				if _, err := client.CreateGroupRelationship(child.ID, parent.ID); err == nil {
					t.Fatal("server accepted a cycle")
				}
				if _, err := client.CreateGroupRelationship(child.ID, child.ID); err == nil {
					t.Fatal("server accepted self membership")
				}
				if err := client.DeleteGroupRelationship(parent.ID, child.ID); err != nil {
					t.Fatal(err)
				}
			}, Config: config, Check: func(*terraform.State) error {
				edge, err := client.GetGroupRelationship(parent.ID, child.ID)
				if err != nil || edge.ParentGroupID != parent.ID || edge.ChildGroupID != child.ID {
					return fmt.Errorf("relationship not recreated: %+v, %v", edge, err)
				}
				return nil
			}},
		},
	})
	_, err := client.GetGroupRelationship(parent.ID, child.ID)
	integrationError(t, err, api.ErrNotFound)
	if _, err := client.GetGroupRelationship(parent.ID, other.ID); err != nil {
		t.Fatalf("destroy removed unrelated fixture relationship: %v", err)
	}
}

func TestIntegrationDirectPermissions(t *testing.T) {
	client := integrationClient(t)
	vault := integrationVault(t)
	group := integrationGroup(t, client)
	account := integrationAccount(t, client)
	for _, target := range []struct {
		name, id, resourceType string
		get                    func(string) ([]api.Permission, error)
		replace                func(string, []api.Permission) ([]api.Permission, error)
	}{
		{"group", group.ID, "rwx_group_permissions", client.GetGroupPermissions, client.ReplaceGroupPermissions},
		{"service_account", account.ID, "rwx_service_account_permissions", client.GetServiceAccountPermissions, client.ReplaceServiceAccountPermissions},
	} {
		t.Run(target.name, func(t *testing.T) {
			catalog, err := client.GetPermissionCatalog(url.Values{target.name + "_id": {target.id}})
			if err != nil || len(catalog.Permissions) == 0 {
				t.Fatalf("permission catalog: %v", err)
			}
			check := func(want []api.Permission) resource.TestCheckFunc {
				return func(*terraform.State) error {
					got, err := target.get(target.id)
					if err != nil || len(got) != len(want) {
						return fmt.Errorf("direct permissions: %+v, %v; want %+v", got, err, want)
					}
					for _, expected := range want {
						found := false
						for _, permission := range got {
							found = found || reflect.DeepEqual(permission, expected)
						}
						if !found {
							return fmt.Errorf("missing direct permission %+v in %+v", expected, got)
						}
					}
					return nil
				}
			}
			config := func(permissions string) string {
				return providerConfig + fmt.Sprintf(`
resource %q "test" {
  %s_id = %q
  permissions = %s
}
`, target.resourceType, target.name, target.id, permissions)
			}
			initial := config(fmt.Sprintf(`[{ key = "run:read" }, { key = "vault:unlock", resource_id = %q }]`, vault.ID))
			want := []api.Permission{{Key: "run:read"}, {Key: "vault:unlock", ResourceID: &vault.ID}}
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{Config: initial, Check: check(want)},
					{ResourceName: target.resourceType + ".test", ImportState: true, ImportStateVerify: true},
					{Config: initial, PlanOnly: true},
					{PreConfig: func() {
						if _, err := target.replace(target.id, []api.Permission{{Key: "not:a:permission"}}); err == nil {
							t.Fatal("server accepted invalid permission")
						}
						if err := check(want)(nil); err != nil {
							t.Fatalf("invalid replacement changed permissions: %v", err)
						}
						if _, err := target.replace(target.id, []api.Permission{}); err != nil {
							t.Fatal(err)
						}
					}, Config: initial, PlanOnly: true, ExpectNonEmptyPlan: true},
					{Config: initial, Check: check(want)},
					{Config: config(`[{ key = "run:read" }]`), Check: check([]api.Permission{{Key: "run:read"}})},
				},
			})
			if err := check(nil)(nil); err != nil {
				t.Fatalf("destroy did not clear direct permissions: %v", err)
			}
		})
	}
}

func TestIntegrationDirectoryAbsence(t *testing.T) {
	client := integrationClient(t)
	directories, err := client.ListDirectories("")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("dedicated org has %d directories; no directory or domain settings changed", len(directories))
	missing := "00000000-0000-4000-8000-000000000000"
	_, err = client.FindDirectoryByName(integrationName())
	integrationError(t, err, api.ErrNotFound)
	_, err = client.GetDirectory(missing)
	integrationError(t, err, api.ErrNotFound)
	_, err = client.FindDirectoryGroupByName(missing, integrationName())
	integrationError(t, err, api.ErrNotFound)
	_, err = client.GetDirectoryGroup(missing, missing)
	integrationError(t, err, api.ErrNotFound)
	_, err = client.GetDirectoryGroupMapping(context.Background(), missing)
	integrationError(t, err, api.ErrNotFound)
	_, err = client.GetDirectoryGroupMappingReconciliation(context.Background(), missing, "")
	integrationError(t, err, api.ErrNotFound)
	_, err = client.CreateDirectoryGroupMapping(context.Background(), api.DirectoryGroupMapping{
		DirectoryID:      missing,
		DirectoryGroupID: missing,
		GroupID:          missing,
	})
	integrationError(t, err, api.ErrNotFound)
	_, err = client.UpdateDirectoryGroupMapping(context.Background(), missing, missing)
	integrationError(t, err, api.ErrNotFound)
	_, err = client.DeleteDirectoryGroupMapping(context.Background(), missing)
	integrationError(t, err, api.ErrNotFound)
}
