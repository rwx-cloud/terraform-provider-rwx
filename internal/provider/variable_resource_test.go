package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestVariableResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + `
resource "rwx_variable" "test" {
  vault = "terraform_provider_testing"
  name  = "test-var"
  value = "foo"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rwx_variable.test", "vault", "terraform_provider_testing"),
					resource.TestCheckResourceAttr("rwx_variable.test", "name", "test-var"),
					resource.TestCheckResourceAttr("rwx_variable.test", "value", "foo"),
				),
			},
			// ImportState testing
			{
				ResourceName:                         "rwx_variable.test",
				ImportState:                          true,
				ImportStateId:                        "terraform_provider_testing/test-var",
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "value",
			},
			// Update and Read testing
			{
				Config: providerConfig + `
resource "rwx_variable" "test" {
  vault = "terraform_provider_testing"
  name  = "test-var"
  value = "bar"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rwx_variable.test", "vault", "terraform_provider_testing"),
					resource.TestCheckResourceAttr("rwx_variable.test", "name", "test-var"),
					resource.TestCheckResourceAttr("rwx_variable.test", "value", "bar"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}
