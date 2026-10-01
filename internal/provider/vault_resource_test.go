package provider

import (
	"fmt"
	"testing"
	"time"

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestVaultResource(t *testing.T) {
	name := fmt.Sprintf("terraform-provider-%d", time.Now().UnixNano())
	updatedName := name + "-updated"
	var vaultID string
	var oidcSubject string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: vaultConfig(name, false, "main"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rwx_vault.test", "name", name),
					resource.TestCheckResourceAttr("rwx_vault.test", "unlocked", "false"),
					resource.TestCheckResourceAttr("rwx_vault.test", "approvals_enabled", "false"),
					resource.TestCheckResourceAttr("rwx_vault.test", "required_approvals", "1"),
					resource.TestCheckResourceAttr("rwx_vault.test", "repository_permissions.#", "1"),
					resource.TestCheckResourceAttrSet("rwx_vault.test", "id"),
					resource.TestCheckResourceAttrSet("rwx_vault.test", "oidc_subject"),
					func(state *terraform.State) error {
						attributes := state.RootModule().Resources["rwx_vault.test"].Primary.Attributes
						vaultID = attributes["id"]
						oidcSubject = attributes["oidc_subject"]
						return nil
					},
				),
			},
			{
				ResourceName:      "rwx_vault.test",
				ImportState:       true,
				ImportStateIdFunc: func(*terraform.State) (string, error) { return vaultID, nil },
				ImportStateVerify: true,
			},
			{
				Config: vaultApprovalConfig(name, false, "main", true, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rwx_vault.test", "approvals_enabled", "true"),
					resource.TestCheckResourceAttr("rwx_vault.test", "required_approvals", "1"),
				),
			},
			{
				Config: vaultApprovalConfig(name, false, "main", true, 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rwx_vault.test", "approvals_enabled", "true"),
					resource.TestCheckResourceAttr("rwx_vault.test", "required_approvals", "2"),
				),
			},
			{
				Config: vaultApprovalConfig(updatedName, true, "release/*", false, 3),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rwx_vault.test", "name", updatedName),
					resource.TestCheckResourceAttr("rwx_vault.test", "unlocked", "true"),
					resource.TestCheckResourceAttr("rwx_vault.test", "approvals_enabled", "false"),
					resource.TestCheckResourceAttr("rwx_vault.test", "required_approvals", "3"),
					resource.TestCheckResourceAttr("rwx_vault.test", "repository_permissions.#", "1"),
					func(state *terraform.State) error {
						attributes := state.RootModule().Resources["rwx_vault.test"].Primary.Attributes
						if attributes["id"] != vaultID {
							return fmt.Errorf("vault ID changed from %q to %q", vaultID, attributes["id"])
						}
						if attributes["oidc_subject"] != oidcSubject {
							return fmt.Errorf("OIDC subject changed from %q to %q", oidcSubject, attributes["oidc_subject"])
						}
						return nil
					},
				),
			},
			{
				PreConfig: func() {
					client := vaultTestClient(t)
					vault, err := client.GetVault(vaultID)
					if err != nil {
						t.Fatal(err)
					}
					vault.Name = name + "-remote"
					vault.ApprovalsEnabled = true
					vault.RequiredApprovals = 4
					if _, err := client.UpdateVault(vault); err != nil {
						t.Fatal(err)
					}
				},
				Config:             vaultApprovalConfig(updatedName, true, "release/*", false, 3),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: vaultApprovalConfig(updatedName, true, "release/*", false, 3),
			},
			{
				Config:   vaultApprovalConfig(updatedName, true, "release/*", false, 3),
				PlanOnly: true,
			},
			{
				PreConfig: func() {
					if err := vaultTestClient(t).DeleteVault(vaultID); err != nil {
						t.Fatal(err)
					}
				},
				Config: vaultApprovalConfig(updatedName, true, "release/*", false, 3),
				Check:  resource.TestCheckResourceAttrSet("rwx_vault.test", "id"),
			},
		},
	})
}

func TestVaultResourceDependenciesAndSelectorMigration(t *testing.T) {
	name := fmt.Sprintf("terraform-provider-dependencies-%d", time.Now().UnixNano())
	secretName := "test-secret"
	var vaultID string
	var secretVersion int

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: vaultDependenciesConfig(name, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rwx_secret.test", "vault", name),
					resource.TestCheckResourceAttr("rwx_variable.test", "vault", name),
					func(state *terraform.State) error {
						vaultID = state.RootModule().Resources["rwx_vault.test"].Primary.Attributes["id"]
						secret, err := vaultTestClient(t).GetSecretMetadataInVault(api.VaultSelector{ID: vaultID}, api.Secret{Name: secretName})
						if err != nil {
							return err
						}
						secretVersion = secret.Version
						return nil
					},
				),
			},
			{
				Config: vaultDependenciesConfig(name, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkResourceAttribute("rwx_secret.test", "vault_id", &vaultID),
					checkResourceAttribute("rwx_variable.test", "vault_id", &vaultID),
					checkSecretVersion(t, &vaultID, secretName, &secretVersion),
				),
			},
			{
				Config: vaultDependenciesConfig(name+"-renamed", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkResourceAttribute("rwx_vault.test", "id", &vaultID),
					checkResourceAttribute("rwx_secret.test", "vault_id", &vaultID),
					checkResourceAttribute("rwx_variable.test", "vault_id", &vaultID),
					checkSecretVersion(t, &vaultID, secretName, &secretVersion),
				),
			},
		},
	})
}

func vaultConfig(name string, unlocked bool, branchPattern string) string {
	return providerConfig + fmt.Sprintf(`
resource "rwx_vault" "test" {
  name     = %q
  unlocked = %t

  repository_permissions = [{
    repository_slug = "rwx-cloud/terraform-provider-rwx"
    branch_pattern  = %q
  }]
}
`, name, unlocked, branchPattern)
}

func vaultApprovalConfig(name string, unlocked bool, branchPattern string, approvalsEnabled bool, requiredApprovals int) string {
	return providerConfig + fmt.Sprintf(`
resource "rwx_vault" "test" {
  name               = %q
  unlocked           = %t
  approvals_enabled  = %t
  required_approvals = %d

  repository_permissions = [{
    repository_slug = "rwx-cloud/terraform-provider-rwx"
    branch_pattern  = %q
  }]
}
`, name, unlocked, approvalsEnabled, requiredApprovals, branchPattern)
}

func vaultDependenciesConfig(name string, useID bool) string {
	selector := "vault = rwx_vault.test.name"
	if useID {
		selector = "vault_id = rwx_vault.test.id"
	}

	return providerConfig + fmt.Sprintf(`
resource "rwx_vault" "test" {
  name = %q
}

resource "rwx_secret" "test" {
  %s
  name         = "test-secret"
  secret_value = "secret-value"
}

resource "rwx_variable" "test" {
  %s
  name  = "test-variable"
  value = "variable-value"
}
`, name, selector, selector)
}

func checkResourceAttribute(resourceName string, attribute string, want *string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		got := state.RootModule().Resources[resourceName].Primary.Attributes[attribute]
		if got != *want {
			return fmt.Errorf("%s.%s = %q, want %q", resourceName, attribute, got, *want)
		}
		return nil
	}
}

func checkSecretVersion(t *testing.T, vaultID *string, secretName string, want *int) resource.TestCheckFunc {
	t.Helper()

	return func(*terraform.State) error {
		secret, err := vaultTestClient(t).GetSecretMetadataInVault(api.VaultSelector{ID: *vaultID}, api.Secret{Name: secretName})
		if err != nil {
			return err
		}
		if secret.Version != *want {
			return fmt.Errorf("secret version changed from %d to %d", *want, secret.Version)
		}
		return nil
	}
}

func vaultTestClient(t *testing.T) api.Client {
	t.Helper()

	return oidcTokenTestClient(t)
}
