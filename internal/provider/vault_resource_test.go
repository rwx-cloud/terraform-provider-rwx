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
				Config: vaultConfig(updatedName, true, "release/*"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rwx_vault.test", "name", updatedName),
					resource.TestCheckResourceAttr("rwx_vault.test", "unlocked", "true"),
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
					if _, err := client.UpdateVault(vault); err != nil {
						t.Fatal(err)
					}
				},
				Config:             vaultConfig(updatedName, true, "release/*"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: vaultConfig(updatedName, true, "release/*"),
			},
			{
				PreConfig: func() {
					if err := vaultTestClient(t).DeleteVault(vaultID); err != nil {
						t.Fatal(err)
					}
				},
				Config: vaultConfig(updatedName, true, "release/*"),
				Check:  resource.TestCheckResourceAttrSet("rwx_vault.test", "id"),
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

func vaultTestClient(t *testing.T) api.Client {
	t.Helper()

	return oidcTokenTestClient(t)
}
