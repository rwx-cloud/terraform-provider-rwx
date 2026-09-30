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

func TestOIDCTokenResource(t *testing.T) {
	name := fmt.Sprintf("terraform-provider-%d", time.Now().UnixNano())
	updatedName := name + "-updated"
	var tokenID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: oidcTokenConfig(name, "initial-audience"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rwx_oidc_token.test", "vault", "terraform_provider_testing"),
					resource.TestCheckResourceAttr("rwx_oidc_token.test", "name", name),
					resource.TestCheckResourceAttr("rwx_oidc_token.test", "audience", "initial-audience"),
					resource.TestCheckResourceAttrSet("rwx_oidc_token.test", "id"),
					resource.TestCheckResourceAttrSet("rwx_oidc_token.test", "subject"),
					resource.TestCheckResourceAttrSet("rwx_oidc_token.test", "expression"),
					func(state *terraform.State) error {
						tokenID = state.RootModule().Resources["rwx_oidc_token.test"].Primary.Attributes["id"]
						if tokenID == "" {
							return fmt.Errorf("OIDC token ID was empty")
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "rwx_oidc_token.test",
				ImportState:       true,
				ImportStateIdFunc: func(*terraform.State) (string, error) { return tokenID, nil },
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					client := oidcTokenTestClient(t)
					token, err := client.FindOIDCToken(tokenID)
					if err != nil {
						t.Fatal(err)
					}
					token.Name = name + "-remote"
					token.Audience = "remote-audience"
					if _, err := client.UpdateOIDCToken(token.Vault.ID, token); err != nil {
						t.Fatal(err)
					}
				},
				Config:             oidcTokenConfig(name, "initial-audience"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: oidcTokenConfig(updatedName, "updated-audience"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rwx_oidc_token.test", "name", updatedName),
					resource.TestCheckResourceAttr("rwx_oidc_token.test", "audience", "updated-audience"),
					func(state *terraform.State) error {
						updatedTokenID := state.RootModule().Resources["rwx_oidc_token.test"].Primary.Attributes["id"]
						if updatedTokenID != tokenID {
							return fmt.Errorf("OIDC token ID changed from %q to %q", tokenID, updatedTokenID)
						}
						return nil
					},
				),
			},
			{
				PreConfig: func() {
					client := oidcTokenTestClient(t)
					token, err := client.FindOIDCToken(tokenID)
					if err != nil {
						t.Fatal(err)
					}
					if err := client.DeleteOIDCToken(token.Vault.ID, token.ID); err != nil {
						t.Fatal(err)
					}
				},
				Config: oidcTokenConfig(updatedName, "updated-audience"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("rwx_oidc_token.test", "id"),
					resource.TestCheckResourceAttr("rwx_oidc_token.test", "name", updatedName),
				),
			},
		},
	})
}

func oidcTokenConfig(name string, audience string) string {
	return providerConfig + fmt.Sprintf(`
resource "rwx_oidc_token" "test" {
  vault    = "terraform_provider_testing"
  name     = %q
  audience = %q
}
`, name, audience)
}

func oidcTokenTestClient(t *testing.T) api.Client {
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
