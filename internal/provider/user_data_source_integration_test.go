package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

func TestIntegrationUserDataSource(t *testing.T) {
	client := integrationClient(t)
	email := os.Getenv("RWX_TERRAFORM_TEST_APPROVER_EMAIL")
	if email == "" {
		t.Fatal("RWX_TERRAFORM_TEST_APPROVER_EMAIL must name an existing active user in rwx-integration-testing")
	}
	user, err := client.FindUserByEmail(email)
	if err != nil {
		t.Fatalf("user fixture lookup failed; requires deployed organization_users API, org:access, and an existing active non-support member in rwx-integration-testing: %v", err)
	}
	if user.ID == "" || user.IdentityID == "" || user.Email == "" {
		t.Fatalf("user fixture lookup returned incomplete user: %+v", user)
	}
	config := providerConfig + fmt.Sprintf(`
data "rwx_user" "by_email" {
  email = %q
}
data "rwx_user" "by_id" {
  id = %q
}
output "email_user_id" { value = data.rwx_user.by_email.id }
output "id_user_id" { value = data.rwx_user.by_id.id }
output "email_identity_id" { value = data.rwx_user.by_email.identity_id }
output "id_identity_id" { value = data.rwx_user.by_id.identity_id }
output "stored_email" { value = data.rwx_user.by_id.email }
`, email, user.ID)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectKnownOutputValue("email_user_id", knownvalue.StringExact(user.ID)),
					plancheck.ExpectKnownOutputValue("id_user_id", knownvalue.StringExact(user.ID)),
					plancheck.ExpectKnownOutputValue("email_identity_id", knownvalue.StringExact(user.IdentityID)),
					plancheck.ExpectKnownOutputValue("id_identity_id", knownvalue.StringExact(user.IdentityID)),
					plancheck.ExpectKnownOutputValue("stored_email", knownvalue.StringExact(user.Email)),
				}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.rwx_user.by_email", "id", user.ID),
					resource.TestCheckResourceAttr("data.rwx_user.by_email", "email", email),
					resource.TestCheckResourceAttr("data.rwx_user.by_id", "id", user.ID),
					resource.TestCheckResourceAttr("data.rwx_user.by_id", "email", user.Email),
				),
			},
			{Config: config, PlanOnly: true},
		},
	})
}
