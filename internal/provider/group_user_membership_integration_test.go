package provider

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"
)

func TestIntegrationGroupUserMembership(t *testing.T) {
	client := integrationClient(t)
	email := os.Getenv("RWX_TERRAFORM_TEST_APPROVER_EMAIL")
	if email == "" {
		t.Fatal("RWX_TERRAFORM_TEST_APPROVER_EMAIL must name an existing user in rwx-integration-testing")
	}
	vault := integrationVault(t)
	approver, err := client.CreateVaultApprover(vault.ID, email)
	if err != nil {
		t.Fatalf("resolve existing test user through disposable vault approver: %v", err)
	}
	t.Cleanup(func() {
		if err := client.DeleteVaultApprover(vault.ID, approver.ID); err != nil {
			t.Errorf("cleanup fixture approver: %v", err)
		}
	})
	userID := approver.User.ID
	if userID == "" || userID == approver.ID || !strings.EqualFold(approver.User.Email, email) {
		t.Fatalf("vault approver did not resolve a distinct existing user ID: %+v", approver)
	}

	group := integrationGroup(t, client)
	child := integrationGroup(t, client)
	other := integrationGroup(t, client)
	if _, err := client.CreateGroupRelationship(group.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.DeleteGroupRelationship(group.ID, child.ID); err != nil && !errors.Is(err, api.ErrNotFound) {
			t.Errorf("cleanup fixture relationship: %v", err)
		}
	})
	for _, groupID := range []string{child.ID, other.ID} {
		if _, err := client.CreateGroupUserMembership(groupID, userID); err != nil {
			t.Fatalf("create fixture membership (requires deployed RWX-2439 API): %v", err)
		}
		t.Cleanup(func() {
			if err := client.DeleteGroupUserMembership(groupID, userID); err != nil && !errors.Is(err, api.ErrNotFound) {
				t.Errorf("cleanup fixture membership: %v", err)
			}
		})
	}
	_, err = client.GetGroupUserMembership(group.ID, userID)
	integrationError(t, err, api.ErrNotFound)
	_, err = client.GetGroupUserMembership(group.ID, approver.ID)
	integrationError(t, err, api.ErrNotFound)
	checkPreserved := func(*terraform.State) error {
		for _, groupID := range []string{child.ID, other.ID} {
			if _, err := client.GetGroupUserMembership(groupID, userID); err != nil {
				return fmt.Errorf("unrelated or inherited-source membership was removed: %w", err)
			}
		}
		if _, err := client.GetGroupRelationship(group.ID, child.ID); err != nil {
			return fmt.Errorf("inherited membership relationship was removed: %w", err)
		}
		return nil
	}
	config := providerConfig + fmt.Sprintf(`
data "rwx_user" "test" {
  email = %q
}

resource "rwx_group_user_membership" "test" {
  group_id = %q
  user_id  = data.rwx_user.test.id
}
`, email, group.ID)
	check := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr("data.rwx_user.test", "id", userID),
		resource.TestCheckResourceAttr("rwx_group_user_membership.test", "id", group.ID+"/"+userID),
		func(*terraform.State) error {
			membership, err := client.GetGroupUserMembership(group.ID, userID)
			if err != nil || membership.GroupID != group.ID || membership.UserID != userID {
				return fmt.Errorf("manual membership not present: %+v, %v", membership, err)
			}
			return nil
		},
		checkPreserved,
	)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config, Check: check},
			{Config: config, PlanOnly: true},
			{ResourceName: "rwx_group_user_membership.test", ImportState: true, ImportStateVerify: true},
			{Config: config, PlanOnly: true},
			{PreConfig: func() {
				if err := client.DeleteGroupUserMembership(group.ID, userID); err != nil {
					t.Fatal(err)
				}
				if err := checkPreserved(nil); err != nil {
					t.Fatal(err)
				}
			}, Config: config, PlanOnly: true, ExpectNonEmptyPlan: true},
			{Config: config, Check: check},
			{Config: config, PlanOnly: true},
		},
	})
	_, err = client.GetGroupUserMembership(group.ID, userID)
	integrationError(t, err, api.ErrNotFound)
	integrationError(t, client.DeleteGroupUserMembership(group.ID, userID), api.ErrNotFound)
	if err := checkPreserved(nil); err != nil {
		t.Fatalf("destroy changed surviving memberships: %v", err)
	}
}
