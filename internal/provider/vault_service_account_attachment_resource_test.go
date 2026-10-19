package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestVaultServiceAccountAttachmentResource(t *testing.T) {
	client := integrationClient(t)
	approverEmail := os.Getenv("RWX_TERRAFORM_TEST_APPROVER_EMAIL")
	if approverEmail == "" {
		t.Fatal("RWX_TERRAFORM_TEST_APPROVER_EMAIL must name an existing user in rwx-integration-testing")
	}

	account := integrationAccount(t, client)
	vault := integrationVault(t)
	config := vaultServiceAccountAttachmentConfig(vault.ID, approverEmail, account.Name)
	var attachmentID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rwx_vault_approver.test", "id"),
					resource.TestCheckResourceAttr("rwx_vault_service_account_attachment.test", "service_account", account.Name),
					resource.TestCheckResourceAttrSet("rwx_vault_service_account_attachment.test", "id"),
					resource.TestCheckResourceAttr("rwx_vault_service_account_attachment.test", "service_account_id", account.ID),
					resource.TestCheckResourceAttrSet("rwx_vault_service_account_attachment.test", "created_at"),
					func(state *terraform.State) error {
						attachmentID = state.RootModule().Resources["rwx_vault_service_account_attachment.test"].Primary.Attributes["id"]
						return nil
					},
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
			{
				ResourceName:      "rwx_vault_service_account_attachment.test",
				ImportState:       true,
				ImportStateIdFunc: func(*terraform.State) (string, error) { return vault.ID + "/" + attachmentID, nil },
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					if err := client.DeleteVaultServiceAccountAttachment(vault.ID, attachmentID); err != nil {
						t.Fatal(err)
					}
				},
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rwx_vault_approver.test", "id"),
					resource.TestCheckResourceAttrSet("rwx_vault_service_account_attachment.test", "id"),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

func vaultServiceAccountAttachmentConfig(vaultID string, approverEmail string, serviceAccount string) string {
	return providerConfig + fmt.Sprintf(`
resource "rwx_vault_approver" "test" {
  vault_id = %q
  email    = %q
}

resource "rwx_vault_service_account_attachment" "test" {
  vault_id        = %q
  service_account = %q
}
`, vaultID, approverEmail, vaultID, serviceAccount)
}
