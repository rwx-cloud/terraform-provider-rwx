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

func TestVaultApproverResource(t *testing.T) {
	approverEmail := os.Getenv("RWX_TERRAFORM_TEST_APPROVER_EMAIL")
	if approverEmail == "" {
		t.Skip("RWX_TERRAFORM_TEST_APPROVER_EMAIL is required")
	}

	vaultName := fmt.Sprintf("terraform-provider-approver-%d", time.Now().UnixNano())
	config := vaultApproverConfig(vaultName, approverEmail)
	var vaultID string
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
						vaultID = state.RootModule().Resources["rwx_vault.test"].Primary.Attributes["id"]
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
				ImportStateIdFunc: func(*terraform.State) (string, error) { return vaultID + "/" + approverID, nil },
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					if err := vaultApproverTestClient(t).DeleteVaultApprover(vaultID, approverID); err != nil {
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

func vaultApproverConfig(vaultName string, approverEmail string) string {
	return providerConfig + fmt.Sprintf(`
resource "rwx_vault" "test" {
  name = %q
}

resource "rwx_vault_approver" "test" {
  vault_id = rwx_vault.test.id
  email    = %q
}
`, vaultName, approverEmail)
}

func vaultApproverTestClient(t *testing.T) api.Client {
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
