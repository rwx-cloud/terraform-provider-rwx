package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"
)

func TestVariableResource(t *testing.T) {
	client := integrationClient(t)
	vault := integrationVault(t)
	t.Cleanup(func() {
		if err := client.DeleteVariableInVault(api.VaultSelector{ID: vault.ID}, api.Variable{Name: "test-var"}); err != nil {
			t.Errorf("cleanup variable in vault %s: %v", vault.ID, err)
		}
	})
	config := func(value string) string {
		return providerConfig + fmt.Sprintf(`
resource "rwx_variable" "test" {
  vault = %q
  name = "test-var"
  value = %q
}
`, vault.Name, value)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rwx_variable.test", "vault", vault.Name),
					resource.TestCheckResourceAttr("rwx_variable.test", "name", "test-var"),
					resource.TestCheckResourceAttr("rwx_variable.test", "value", "foo"),
				),
			},
			{
				ResourceName:                         "rwx_variable.test",
				ImportState:                          true,
				ImportStateId:                        vault.Name + "/test-var",
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "value",
			},
			{
				Config: config("bar"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rwx_variable.test", "vault", vault.Name),
					resource.TestCheckResourceAttr("rwx_variable.test", "name", "test-var"),
					resource.TestCheckResourceAttr("rwx_variable.test", "value", "bar"),
				),
			},
		},
	})
	_, err := client.GetVariableInVault(api.VaultSelector{ID: vault.ID}, api.Variable{Name: "test-var"})
	integrationError(t, err, api.ErrNotFound)
}
