package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"
)

func TestIntegrationGroupServiceAccountMembership(t *testing.T) {
	client := integrationClient(t)
	parent := integrationGroup(t, client)
	child := integrationGroup(t, client)
	other := integrationGroup(t, client)
	account := integrationAccount(t, client)
	otherAccount := integrationAccount(t, client)
	if _, err := client.CreateGroupRelationship(parent.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{child.ID, account.ID}, {other.ID, account.ID}, {parent.ID, otherAccount.ID}} {
		if _, err := client.CreateGroupServiceAccountMembership(pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}
	_, err := client.GetGroupServiceAccountMembership(parent.ID, account.ID)
	integrationError(t, err, api.ErrNotFound)
	config := providerConfig + fmt.Sprintf(`
resource "rwx_group_service_account_membership" "test" {
  group_id = %q
  service_account_id = %q
}
`, parent.ID, account.ID)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config, Check: resource.TestCheckResourceAttr("rwx_group_service_account_membership.test", "id", parent.ID+"/"+account.ID)},
			{Config: config, PlanOnly: true},
			{ResourceName: "rwx_group_service_account_membership.test", ImportState: true, ImportStateVerify: true},
			{Config: config, PlanOnly: true},
			{PreConfig: func() {
				if err := client.DeleteGroupServiceAccountMembership(parent.ID, account.ID); err != nil {
					t.Fatal(err)
				}
			}, Config: config, PlanOnly: true, ExpectNonEmptyPlan: true},
			{Config: config, Check: func(*terraform.State) error {
				membership, err := client.GetGroupServiceAccountMembership(parent.ID, account.ID)
				if err != nil || membership.GroupID != parent.ID || membership.ServiceAccountID != account.ID {
					return fmt.Errorf("membership not recreated: %+v, %v", membership, err)
				}
				return nil
			}},
		},
	})
	_, err = client.GetGroupServiceAccountMembership(parent.ID, account.ID)
	integrationError(t, err, api.ErrNotFound)
	for _, pair := range [][2]string{{child.ID, account.ID}, {other.ID, account.ID}, {parent.ID, otherAccount.ID}} {
		if _, err := client.GetGroupServiceAccountMembership(pair[0], pair[1]); err != nil {
			t.Fatalf("destroy removed unrelated membership: %v", err)
		}
	}
	if _, err := client.GetGroupRelationship(parent.ID, child.ID); err != nil {
		t.Fatalf("destroy removed inherited-access path: %v", err)
	}
}
