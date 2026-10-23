package api

import (
	"context"
	"net/http"
	"testing"
)

func TestTerraformManagementIntent(t *testing.T) {
	result := func(_ any, err error) error { return err }
	ctx := context.Background()
	for _, tc := range []struct {
		name, intent string
		status       int
		call         func(Client) error
	}{
		{"create group", "claim", 201, func(c Client) error { return result(c.CreateGroup(Group{})) }},
		{"update group", "claim", 200, func(c Client) error { return result(c.UpdateGroup(Group{ID: "group-one"})) }},
		{"create account", "claim", 201, func(c Client) error { return result(c.CreateServiceAccount(ServiceAccount{})) }},
		{"update account", "claim", 200, func(c Client) error { return result(c.UpdateServiceAccount(ServiceAccount{ID: "account-one"})) }},
		{"create vault", "claim", 201, func(c Client) error { return result(c.CreateVault(Vault{})) }},
		{"update vault", "claim", 200, func(c Client) error { return result(c.UpdateVault(Vault{ID: "vault-one"})) }},
		{"upsert secret", "claim", 200, func(c Client) error {
			return result(c.SetSecretInVault(VaultSelector{ID: "vault-one"}, Secret{Name: "token"}))
		}},
		{"upsert variable", "claim", 200, func(c Client) error {
			return result(c.SetVariableInVault(VaultSelector{ID: "vault-one"}, Variable{Name: "region"}))
		}},
		{"create OIDC definition", "claim", 201, func(c Client) error { return result(c.CreateOIDCToken(VaultSelector{ID: "vault-one"}, OIDCToken{})) }},
		{"update OIDC definition", "claim", 200, func(c Client) error { return result(c.UpdateOIDCToken("vault-one", OIDCToken{ID: "token-one"})) }},
		{"create approver", "claim", 201, func(c Client) error { return result(c.CreateVaultApprover("vault-one", "user@example.com")) }},
		{"create attachment", "claim", 201, func(c Client) error { return result(c.CreateVaultServiceAccountAttachment("vault-one", "deploys")) }},
		{"create user access grant", "claim", 201, func(c Client) error {
			return result(c.CreateAccessGrant("vault-one", AccessGrantPrincipal{Type: AccessGrantPrincipalTypeUser, Email: "user@example.com"}, nil))
		}},
		{"create account access grant", "claim", 201, func(c Client) error {
			return result(c.CreateAccessGrant("vault-one", AccessGrantPrincipal{Type: AccessGrantPrincipalTypeServiceAccount, Name: "deploys"}, nil))
		}},
		{"update access grant", "claim", 200, func(c Client) error { return result(c.UpdateAccessGrant("vault-one", "grant-one", nil)) }},
		{"replace group permissions", "claim", 200, func(c Client) error { return result(c.ReplaceGroupPermissions("group-one", nil)) }},
		{"replace account permissions", "claim", 200, func(c Client) error { return result(c.ReplaceServiceAccountPermissions("account-one", nil)) }},
		{"clear group permissions", "release", 200, func(c Client) error { return c.ClearGroupPermissions("group-one") }},
		{"clear account permissions", "release", 200, func(c Client) error { return c.ClearServiceAccountPermissions("account-one") }},
		{"create relationship", "claim", 201, func(c Client) error { return result(c.CreateGroupRelationship("parent", "child")) }},
		{"create user membership", "claim", 201, func(c Client) error { return result(c.CreateGroupUserMembership("group-one", "user-two")) }},
		{"create account membership", "claim", 201, func(c Client) error { return result(c.CreateGroupServiceAccountMembership("group-one", "account-two")) }},
		{"create mapping", "claim", 200, func(c Client) error { return result(c.CreateDirectoryGroupMapping(ctx, DirectoryGroupMapping{})) }},
		{"update mapping", "claim", 200, func(c Client) error { return result(c.UpdateDirectoryGroupMapping(ctx, "mapping-one", "group-two")) }},
		{"read group", "", 200, func(c Client) error { return result(c.GetGroup("group-one")) }},
		{"read account", "", 200, func(c Client) error { return result(c.GetServiceAccount("account-one")) }},
		{"read vault", "", 200, func(c Client) error { return result(c.GetVault("vault-one")) }},
		{"read secret metadata", "", 200, func(c Client) error {
			return result(c.GetSecretMetadataInVault(VaultSelector{ID: "vault-one"}, Secret{Name: "token"}))
		}},
		{"read variable", "", 200, func(c Client) error {
			return result(c.GetVariableInVault(VaultSelector{ID: "vault-one"}, Variable{Name: "region"}))
		}},
		{"read OIDC definition", "", 200, func(c Client) error { return result(c.GetOIDCToken("vault-one", "token-one")) }},
		{"read approver", "", 200, func(c Client) error { return result(c.GetVaultApprover("vault-one", "approver-one")) }},
		{"read attachment", "", 200, func(c Client) error { return result(c.GetVaultServiceAccountAttachment("vault-one", "attachment-one")) }},
		{"read access grant", "", 200, func(c Client) error { return result(c.GetAccessGrant("vault-one", "grant-one")) }},
		{"list access grants", "", 200, func(c Client) error { return result(c.ListAccessGrants("vault-one")) }},
		{"read group permissions", "", 200, func(c Client) error { return result(c.GetGroupPermissions("group-one")) }},
		{"read account permissions", "", 200, func(c Client) error { return result(c.GetServiceAccountPermissions("account-one")) }},
		{"read relationship", "", 200, func(c Client) error { return result(c.GetGroupRelationship("parent", "child")) }},
		{"read user membership", "", 200, func(c Client) error { return result(c.GetGroupUserMembership("group-one", "user-two")) }},
		{"read account membership", "", 200, func(c Client) error { return result(c.GetGroupServiceAccountMembership("group-one", "account-two")) }},
		{"read mapping", "", 200, func(c Client) error { return result(c.GetDirectoryGroupMapping(ctx, "mapping-one")) }},
		{"delete group", "", 204, func(c Client) error { return c.DeleteGroup("group-one") }},
		{"delete account", "", 204, func(c Client) error { return c.DeleteServiceAccount("account-one") }},
		{"delete vault", "", 204, func(c Client) error { return c.DeleteVault("vault-one") }},
		{"delete secret", "", 200, func(c Client) error {
			return c.DeleteSecretInVault(VaultSelector{ID: "vault-one"}, Secret{Name: "token"})
		}},
		{"delete variable", "", 200, func(c Client) error {
			return c.DeleteVariableInVault(VaultSelector{ID: "vault-one"}, Variable{Name: "region"})
		}},
		{"delete OIDC definition", "", 200, func(c Client) error { return c.DeleteOIDCToken("vault-one", "token-one") }},
		{"delete approver", "", 204, func(c Client) error { return c.DeleteVaultApprover("vault-one", "approver-one") }},
		{"delete attachment", "", 204, func(c Client) error { return c.DeleteVaultServiceAccountAttachment("vault-one", "attachment-one") }},
		{"delete access grant", "", 204, func(c Client) error { return c.DeleteAccessGrant("vault-one", "grant-one") }},
		{"delete mapping", "", 200, func(c Client) error { return result(c.DeleteDirectoryGroupMapping(ctx, "mapping-one")) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
				if got := req.Header.Get("X-RWX-Terraform-Management"); got != tc.intent {
					t.Fatalf("management intent = %q, want %q", got, tc.intent)
				}
				return jsonResponse(tc.status, `{"permissions":[],"versions":{"token":7}}`), nil
			}}
			if err := tc.call(client); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClaimTerraformManagementEscaping(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.EscapedPath() != "/mint/api/groups/group%2Fone/terraform_management" || req.URL.Query().Get("key") != "value&two" {
			t.Fatalf("incorrect claim scope: %s %s", req.Method, req.URL)
		}
		return jsonResponse(204, ""), nil
	}}
	if err := client.ClaimTerraformManagement(context.Background(), "/mint/api/groups/group%2Fone?key=value%26two"); err != nil {
		t.Fatal(err)
	}
}
