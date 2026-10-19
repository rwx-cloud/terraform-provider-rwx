package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestVaultApproverResource(t *testing.T) {
	client := integrationClient(t)
	approverEmail := os.Getenv("RWX_TERRAFORM_TEST_APPROVER_EMAIL")
	if approverEmail == "" {
		t.Fatal("RWX_TERRAFORM_TEST_APPROVER_EMAIL must name an existing user in rwx-integration-testing")
	}

	vault := integrationVault(t)
	config := vaultApproverConfig(vault.ID, approverEmail)
	var approverID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rwx_vault_approver.test", "email", approverEmail),
					resource.TestCheckResourceAttrSet("rwx_vault_approver.test", "id"),
					resource.TestCheckResourceAttrSet("rwx_vault_approver.test", "user_id"),
					func(state *terraform.State) error {
						approverID = state.RootModule().Resources["rwx_vault_approver.test"].Primary.Attributes["id"]
						return nil
					},
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
			{
				ResourceName:      "rwx_vault_approver.test",
				ImportState:       true,
				ImportStateIdFunc: func(*terraform.State) (string, error) { return vault.ID + "/" + approverID, nil },
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					if err := client.DeleteVaultApprover(vault.ID, approverID); err != nil {
						t.Fatal(err)
					}
				},
				Config: config,
				Check:  resource.TestCheckResourceAttrSet("rwx_vault_approver.test", "id"),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

func vaultApproverConfig(vaultID string, approverEmail string) string {
	return providerConfig + fmt.Sprintf(`
resource "rwx_vault_approver" "test" {
  vault_id = %q
  email    = %q
}
`, vaultID, approverEmail)
}
