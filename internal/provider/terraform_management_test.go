package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"
)

func TestResourceReadClaimsTerraformManagement(t *testing.T) {
	cases := []struct {
		name, id, endpoint, query, body string
		makeResource                    func() resource.Resource
	}{
		{"group", "group-one", "/mint/api/groups/group-one", "", `{"group":{"id":"group-one","name":"Deployers"}}`, NewGroupResource},
		{"service_account", "account-one", "/mint/api/service_accounts/account-one", "", `{"service_account":{"id":"account-one","name":"deploys","display_name":"Deploys","identity_id":"identity-one"}}`, NewServiceAccountResource},
		{"vault", "vault-one", "/mint/api/vaults/vault-one", "", `{"vault":{"id":"vault-one","name":"deploys","lock_status":"locked","required_approvals":1,"repository_permissions":[],"oidc_subject":"subject-one"}}`, NewVaultResource},
		{"group_permissions", "group-one", "/mint/api/groups/group-one/permissions", "", `{"permissions":[]}`, NewGroupPermissionsResource},
		{"service_account_permissions", "account-one", "/mint/api/service_accounts/account-one/permissions", "", `{"permissions":[]}`, NewServiceAccountPermissionsResource},
		{"group_relationship", "parent/child", "/mint/api/group_relationships", "child_group_id=child&parent_group_id=parent", `{"group_relationship":{"parent_group_id":"parent","child_group_id":"child"}}`, NewGroupRelationshipResource},
		{"group_user_membership", "group-one/user-two", "/mint/api/group_user_memberships", "group_id=group-one&user_id=user-two", `{"group_user_membership":{"group_id":"group-one","user_id":"user-two"}}`, NewGroupUserMembershipResource},
		{"group_service_account_membership", "group-one/account-two", "/mint/api/group_service_account_memberships", "group_id=group-one&service_account_id=account-two", `{"group_service_account_membership":{"group_id":"group-one","service_account_id":"account-two"}}`, NewGroupServiceAccountMembershipResource},
		{"directory_group_mapping", "directory-group-one", "/mint/api/directory_group_mappings/directory-group-one", "", `{"directory_group_mapping":{"id":"directory-group-one","directory_id":"directory-one","directory_group_id":"directory-group-one","group_id":"group-two"},"reconciliation":{"status":"succeeded"}}`, NewDirectoryGroupMappingResource},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, status := range []int{204, 401, 403, 404, 422, 500} {
				t.Run(fmt.Sprint(status), func(t *testing.T) {
					ctx := context.Background()
					claimed := false
					missing := false
					client := api.Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
						if req.URL.RawQuery != tc.query || req.Header.Get(api.TerraformManagementHeader) != "" {
							t.Fatalf("unexpected claim scope/intent: %s %v", req.URL, req.Header)
						}
						code, body := 200, tc.body
						switch {
						case req.Method == http.MethodGet && req.URL.Path == tc.endpoint:
							if missing {
								code, body = 404, `{"errors":["missing"]}`
							}
						case req.Method == http.MethodPost && req.URL.Path == tc.endpoint+"/terraform_management":
							if missing {
								t.Fatal("claimed a missing record")
							}
							code, body = status, `{"errors":["claim rejected"]}`
							claimed = status == 204
						default:
							t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
						}
						return &http.Response{StatusCode: code, Status: http.StatusText(code), Body: io.NopCloser(strings.NewReader(body))}, nil
					}}
					r := tc.makeResource()
					var configured resource.ConfigureResponse
					r.(resource.ResourceWithConfigure).Configure(ctx, resource.ConfigureRequest{ProviderData: client}, &configured)
					var schema resource.SchemaResponse
					r.Schema(ctx, resource.SchemaRequest{}, &schema)
					imported := resource.ImportStateResponse{State: tfsdk.State{Schema: schema.Schema, Raw: tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), nil)}}
					r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: tc.id}, &imported)
					if imported.Diagnostics.HasError() || configured.Diagnostics.HasError() {
						t.Fatalf("configure/import: %v %v", configured.Diagnostics, imported.Diagnostics)
					}
					response := resource.ReadResponse{State: imported.State}
					r.Read(ctx, resource.ReadRequest{State: imported.State}, &response)
					if status != 204 {
						if !response.Diagnostics.HasError() || !response.State.Raw.Equal(imported.State.Raw) {
							t.Fatalf("claim failure lost state or diagnostics: %v %v", response.State, response.Diagnostics)
						}
						return
					}
					var id string
					diags := response.State.GetAttribute(ctx, path.Root("id"), &id)
					if response.Diagnostics.HasError() || diags.HasError() || !claimed || id != tc.id {
						t.Fatalf("Read did not claim imported resource: claimed=%t id=%s diagnostics=%v %v", claimed, id, response.Diagnostics, diags)
					}
					claimed = false
					again := resource.ReadResponse{State: response.State}
					r.Read(ctx, resource.ReadRequest{State: response.State}, &again)
					if again.Diagnostics.HasError() || !claimed || !again.State.Raw.Equal(response.State.Raw) {
						t.Fatalf("unchanged refresh did not restore management attribution: %v", again.Diagnostics)
					}
					missing = true
					absent := resource.ReadResponse{State: response.State}
					r.Read(ctx, resource.ReadRequest{State: response.State}, &absent)
					if absent.Diagnostics.HasError() || !absent.State.Raw.IsNull() {
						t.Fatalf("missing resource retained state: %v", absent.Diagnostics)
					}
				})
			}
		})
	}
}

func TestVaultChildReadClaimsTerraformManagement(t *testing.T) {
	for _, tc := range []struct {
		typeName, importID, endpoint, query, body, attribute, value string
	}{
		{"rwx_secret", "production/token", "/mint/api/vaults/secrets/token", "vault_id=vault-two", `{"name":"token","description":"deploy token","version":7}`, "secret_value", ""},
		{"rwx_secret", "vault_id/vault-two/token", "/mint/api/vaults/secrets/token", "vault_id=vault-two", `{"name":"token","description":"deploy token","version":7}`, "description", "deploy token"},
		{"rwx_variable", "production/region", "/mint/api/vaults/vars/region", "vault_id=vault-two", `{"name":"region","value":"us-east-2"}`, "value", "us-east-2"},
		{"rwx_variable", "vault_id/vault-two/region", "/mint/api/vaults/vars/region", "vault_id=vault-two", `{"name":"region","value":"us-east-2"}`, "value", "us-east-2"},
		{"rwx_oidc_token", "token-two", "/mint/api/vaults/vault-two/oidc_tokens/token-two", "", `{"id":"token-two","vault":{"id":"vault-two","name":"production"},"name":"deploy","audience":"aws","subject":"subject","expression":"expression"}`, "id", "token-two"},
		{"rwx_vault_approver", "vault-two/approver-two", "/mint/api/vaults/vault-two/approvers/approver-two", "", `{"approver":{"id":"approver-two","vault":{"id":"vault-two","name":"production"},"user":{"id":"user-three","email":"user@example.com"}}}`, "user_id", "user-three"},
		{"rwx_vault_service_account_attachment", "vault-two/attachment-two", "/mint/api/vaults/vault-two/service_account_attachments/attachment-two", "", `{"service_account_attachment":{"id":"attachment-two","vault":{"id":"vault-two","name":"production"},"service_account":{"id":"account-three","name":"deploys"},"created_at":"2026-01-02T03:04:05Z"}}`, "service_account_id", "account-three"},
		{"rwx_vault_user_access_grant", "grant-two", "/mint/api/vaults/vault-two/access_grants/grant-two", "", `{"access_grant":{"id":"grant-two","vault":{"id":"vault-two","name":"production"},"principal":{"type":"user","id":"user-three","email":"user@example.com"},"expires_at":null}}`, "email", "user@example.com"},
		{"rwx_vault_service_account_access_grant", "grant-three", "/mint/api/vaults/vault-two/access_grants/grant-three", "", `{"access_grant":{"id":"grant-three","vault":{"id":"vault-two","name":"production"},"principal":{"type":"service_account","id":"account-three","name":"deploys"},"expires_at":null}}`, "service_account", "deploys"},
	} {
		t.Run(tc.typeName+"/"+tc.importID, func(t *testing.T) {
			claimStatus, claims, getStatus := 204, 0, 200
			mutations := 0
			wrongPrincipal := false
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if (req.Method == http.MethodGet || strings.HasSuffix(req.URL.Path, "/terraform_management")) && req.Header.Get(api.TerraformManagementHeader) != "" {
					t.Errorf("unexpected mutation intent on %s %s", req.Method, req.URL)
				}
				switch {
				case req.Method == http.MethodGet && req.URL.Path == "/mint/api/vaults":
					if req.URL.Query().Get("cursor") == "next" {
						_, _ = fmt.Fprint(w, `{"vaults":[{"id":"vault-two","name":"production"}]}`)
					} else {
						_, _ = fmt.Fprint(w, `{"vaults":[{"id":"vault-one","name":"other"}],"pagination":{"next_cursor":"next"}}`)
					}
				case req.Method == http.MethodGet && req.URL.Path == tc.endpoint && req.URL.RawQuery == tc.query:
					w.WriteHeader(getStatus)
					body := tc.body
					if wrongPrincipal {
						body = strings.Replace(body, `"type":"user"`, `"type":"service_account"`, 1)
					}
					_, _ = fmt.Fprint(w, body)
				case req.Method == http.MethodPost && req.URL.Path == tc.endpoint+"/terraform_management" && req.URL.RawQuery == tc.query:
					claims++
					w.WriteHeader(claimStatus)
				case req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/mint/api/vaults/vault-one/"):
					w.WriteHeader(http.StatusNotFound)
				case req.Method == http.MethodPatch && req.URL.Path == tc.endpoint,
					req.Method == http.MethodPost && (req.URL.Path == "/mint/api/vaults/secrets" || req.URL.Path == "/mint/api/vaults/vars"):
					mutations++
					if req.Header.Get(api.TerraformManagementHeader) != "claim" {
						t.Error("mutation omitted management intent")
					}
					w.WriteHeader(http.StatusBadRequest)
					_, _ = fmt.Fprint(w, `{"versions":{"other-secret":3},"error_messages":[{"message":"requested mutation rejected"}]}`)
				default:
					t.Errorf("unexpected request: %s %s", req.Method, req.URL)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()
			previousClient := http.DefaultClient
			http.DefaultClient = server.Client()
			defer func() { http.DefaultClient = previousClient }()
			ctx := context.Background()
			protocol := providerserver.NewProtocol6(New("test")())()
			schema, err := protocol.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
			if err != nil {
				t.Fatal(err)
			}
			config, err := tfprotov6.NewDynamicValue(schema.Provider.ValueType(), tftypes.NewValue(schema.Provider.ValueType(), map[string]tftypes.Value{
				"host":         tftypes.NewValue(tftypes.String, strings.TrimPrefix(server.URL, "https://")),
				"access_token": tftypes.NewValue(tftypes.String, "test-token"),
			}))
			if err != nil {
				t.Fatal(err)
			}
			configured, err := protocol.ConfigureProvider(ctx, &tfprotov6.ConfigureProviderRequest{Config: &config})
			if err != nil || len(configured.Diagnostics) != 0 {
				t.Fatalf("configure: %v %v", err, configured.Diagnostics)
			}
			imported, err := protocol.ImportResourceState(ctx, &tfprotov6.ImportResourceStateRequest{TypeName: tc.typeName, ID: tc.importID})
			if err != nil || len(imported.Diagnostics) != 0 || len(imported.ImportedResources) != 1 || claims != 0 {
				t.Fatalf("import: %v %v claims=%d", err, imported, claims)
			}
			current := imported.ImportedResources[0].State
			private := imported.ImportedResources[0].Private
			resourceType := schema.ResourceSchemas[tc.typeName].ValueType()
			claimStatus = 403
			failedImport, err := protocol.ReadResource(ctx, &tfprotov6.ReadResourceRequest{TypeName: tc.typeName, CurrentState: current, Private: private})
			if err != nil {
				t.Fatal(err)
			}
			importedValue, err := current.Unmarshal(resourceType)
			if err != nil {
				t.Fatal(err)
			}
			failedValue, err := failedImport.NewState.Unmarshal(resourceType)
			if err != nil || len(failedImport.Diagnostics) == 0 || !failedValue.Equal(importedValue) {
				t.Fatalf("failed post-import claim lost state: %v diagnostics=%v before=%v after=%v", err, failedImport.Diagnostics, importedValue, failedValue)
			}
			claimStatus, claims = 204, 0
			for i := 0; i < 2; i++ {
				read, err := protocol.ReadResource(ctx, &tfprotov6.ReadResourceRequest{TypeName: tc.typeName, CurrentState: current, Private: private})
				if err != nil || len(read.Diagnostics) != 0 || claims != i+1 {
					t.Fatalf("read: %v %v claims=%d", err, read.Diagnostics, claims)
				}
				value, err := read.NewState.Unmarshal(resourceType)
				if err != nil {
					t.Fatal(err)
				}
				var attributes map[string]tftypes.Value
				if err := value.As(&attributes); err != nil {
					t.Fatal(err)
				}
				if !attributes[tc.attribute].Equal(tftypes.NewValue(tftypes.String, tc.value)) {
					t.Fatalf("unexpected refreshed %s: %v", tc.attribute, attributes[tc.attribute])
				}
				if i == 1 && (!bytes.Equal(read.NewState.MsgPack, current.MsgPack) || !bytes.Equal(read.Private, private)) {
					t.Fatal("unchanged refresh altered state")
				}
				current, private = read.NewState, read.Private
			}
			if !strings.Contains(tc.typeName, "access_grant") {
				value, err := current.Unmarshal(resourceType)
				if err != nil {
					t.Fatal(err)
				}
				var attributes map[string]tftypes.Value
				if err := value.As(&attributes); err != nil {
					t.Fatal(err)
				}
				attributes["vault"] = tftypes.NewValue(tftypes.String, nil)
				attributes["vault_id"] = tftypes.NewValue(tftypes.String, "vault-two")
				plan, err := tfprotov6.NewDynamicValue(resourceType, tftypes.NewValue(resourceType, attributes))
				if err != nil {
					t.Fatal(err)
				}
				for _, status := range []int{403, 204} {
					claimStatus = status
					previousClaims := claims
					updated, err := protocol.ApplyResourceChange(ctx, &tfprotov6.ApplyResourceChangeRequest{TypeName: tc.typeName, PriorState: current, PlannedState: &plan, Config: &plan, PlannedPrivate: private})
					if err != nil || claims != previousClaims+1 {
						t.Fatalf("selector update did not claim: %v %v claims=%d", err, updated, claims)
					}
					if status == 403 {
						if len(updated.Diagnostics) == 0 || !bytes.Equal(updated.NewState.MsgPack, current.MsgPack) || !bytes.Equal(updated.Private, private) {
							t.Fatalf("failed selector update lost state: %v", updated)
						}
					} else {
						updatedValue, err := updated.NewState.Unmarshal(resourceType)
						if err != nil || len(updated.Diagnostics) != 0 || !updatedValue.Equal(tftypes.NewValue(resourceType, attributes)) {
							t.Fatalf("selector update failed: %v %v", err, updated)
						}
						current, private = updated.NewState, updated.Private
					}
				}
			}
			mutationAttribute := map[string]string{
				"rwx_secret": "secret_value", "rwx_variable": "value", "rwx_oidc_token": "audience",
				"rwx_vault_user_access_grant": "expires_at", "rwx_vault_service_account_access_grant": "expires_at",
			}[tc.typeName]
			if mutationAttribute != "" {
				value, err := current.Unmarshal(resourceType)
				if err != nil {
					t.Fatal(err)
				}
				var attributes map[string]tftypes.Value
				if err := value.As(&attributes); err != nil {
					t.Fatal(err)
				}
				attributes[mutationAttribute] = tftypes.NewValue(tftypes.String, "2099-01-01T00:00:00Z")
				plan, err := tfprotov6.NewDynamicValue(resourceType, tftypes.NewValue(resourceType, attributes))
				if err != nil {
					t.Fatal(err)
				}
				previousClaims := claims
				updated, err := protocol.ApplyResourceChange(ctx, &tfprotov6.ApplyResourceChangeRequest{TypeName: tc.typeName, PriorState: current, PlannedState: &plan, Config: &plan, PlannedPrivate: private})
				if err != nil || len(updated.Diagnostics) == 0 || mutations != 1 || claims != previousClaims || !bytes.Equal(updated.NewState.MsgPack, current.MsgPack) || !bytes.Equal(updated.Private, private) {
					t.Fatalf("failed mutation lost state or attribution intent: %v %v", err, updated)
				}
			}
			for _, status := range []int{401, 403, 404, 422, 500} {
				claimStatus = status
				read, err := protocol.ReadResource(ctx, &tfprotov6.ReadResourceRequest{TypeName: tc.typeName, CurrentState: current, Private: private})
				if err != nil || len(read.Diagnostics) == 0 || !bytes.Equal(read.NewState.MsgPack, current.MsgPack) || !bytes.Equal(read.Private, private) {
					t.Fatalf("claim %d lost state/diagnostics: %v %v", status, err, read)
				}
			}
			previousClaims := claims
			if tc.typeName == "rwx_vault_user_access_grant" {
				wrongPrincipal = true
				read, err := protocol.ReadResource(ctx, &tfprotov6.ReadResourceRequest{TypeName: tc.typeName, CurrentState: current, Private: private})
				if err != nil || len(read.Diagnostics) == 0 || claims != previousClaims || !bytes.Equal(read.NewState.MsgPack, current.MsgPack) {
					t.Fatalf("mismatched grant principal was claimed or lost state: %v %v", err, read)
				}
			}
			getStatus = 404
			absent, err := protocol.ReadResource(ctx, &tfprotov6.ReadResourceRequest{TypeName: tc.typeName, CurrentState: current, Private: private})
			if err != nil || len(absent.Diagnostics) != 0 || claims != previousClaims {
				t.Fatalf("missing child claimed: %v %v", err, absent)
			}
			value, err := absent.NewState.Unmarshal(resourceType)
			if err != nil || !value.IsNull() {
				t.Fatalf("missing child retained state: %v %v", value, err)
			}
		})
	}
}
