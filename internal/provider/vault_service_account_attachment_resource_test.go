package provider

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestVaultServiceAccountAttachmentResource(t *testing.T) {
	approverEmail := os.Getenv("RWX_TERRAFORM_TEST_APPROVER_EMAIL")
	serviceAccount := os.Getenv("RWX_TERRAFORM_TEST_SERVICE_ACCOUNT")
	if approverEmail == "" || serviceAccount == "" {
		t.Skip("RWX_TERRAFORM_TEST_APPROVER_EMAIL and RWX_TERRAFORM_TEST_SERVICE_ACCOUNT are required")
	}

	vaultName := fmt.Sprintf("terraform-provider-attachment-%d", time.Now().UnixNano())
	config := vaultServiceAccountAttachmentConfig(vaultName, approverEmail, serviceAccount)
	var vaultID string
	var attachmentID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rwx_vault_approver.test", "id"),
					resource.TestCheckResourceAttr("rwx_vault_service_account_attachment.test", "service_account", serviceAccount),
					resource.TestCheckResourceAttrSet("rwx_vault_service_account_attachment.test", "id"),
					resource.TestCheckResourceAttrSet("rwx_vault_service_account_attachment.test", "service_account_id"),
					resource.TestCheckResourceAttrSet("rwx_vault_service_account_attachment.test", "created_at"),
					func(state *terraform.State) error {
						vaultID = state.RootModule().Resources["rwx_vault.test"].Primary.Attributes["id"]
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
				ImportStateIdFunc: func(*terraform.State) (string, error) { return vaultID + "/" + attachmentID, nil },
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					if err := vaultServiceAccountAttachmentTestClient(t).DeleteVaultServiceAccountAttachment(vaultID, attachmentID); err != nil {
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

func vaultServiceAccountAttachmentConfig(vaultName string, approverEmail string, serviceAccount string) string {
	return providerConfig + fmt.Sprintf(`
resource "rwx_vault" "test" {
  name = %q
}

resource "rwx_vault_approver" "test" {
  vault_id = rwx_vault.test.id
  email    = %q
}

resource "rwx_vault_service_account_attachment" "test" {
  vault_id        = rwx_vault.test.id
  service_account = %q
}
`, vaultName, approverEmail, serviceAccount)
}

func vaultServiceAccountAttachmentTestClient(t *testing.T) api.Client {
	t.Helper()

	host := os.Getenv("RWX_HOST")
	if host == "" {
		host = "cloud.rwx.com"
	}
	client, err := api.NewClient(api.Config{
		Host:        host,
		AccessToken: os.Getenv("RWX_ACCESS_TOKEN"),
		Version:     "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	return client
}
