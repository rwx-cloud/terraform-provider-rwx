package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestSecretResource(t *testing.T) {
	client := integrationClient(t)
	vault := integrationVault(t)
	t.Cleanup(func() {
		if err := client.DeleteSecretInVault(api.VaultSelector{ID: vault.ID}, api.Secret{Name: "test-secret"}); err != nil {
			t.Errorf("cleanup secret in vault %s: %v", vault.ID, err)
		}
	})
	config := func(value, description string) string {
		return providerConfig + fmt.Sprintf(`
resource "rwx_secret" "test" {
  vault = %q
  name = "test-secret"
  secret_value = %q
  %s
}
`, vault.Name, value, description)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("foo", `description = "a description"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rwx_secret.test", "vault", vault.Name),
					resource.TestCheckResourceAttr("rwx_secret.test", "name", "test-secret"),
					resource.TestCheckResourceAttr("rwx_secret.test", "secret_value", "foo"),
					resource.TestCheckResourceAttr("rwx_secret.test", "description", "a description"),
				),
			},
			{
				Config: config("bar", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rwx_secret.test", "vault", vault.Name),
					resource.TestCheckResourceAttr("rwx_secret.test", "name", "test-secret"),
					resource.TestCheckResourceAttr("rwx_secret.test", "secret_value", "bar"),
					resource.TestCheckNoResourceAttr("rwx_secret.test", "description"),
				),
			},
		},
	})
	_, err := client.GetSecretMetadataInVault(api.VaultSelector{ID: vault.ID}, api.Secret{Name: "test-secret"})
	integrationError(t, err, api.ErrNotFound)
}

func TestSecretResourceImport(t *testing.T) {
	for _, selector := range []string{"vault", "vault_id"} {
		for _, workflow := range []string{"command", "block"} {
			t.Run(selector+"/"+workflow, func(t *testing.T) {
				var mu sync.Mutex
				secret := api.Secret{Name: "existing-secret", SecretValue: "original", Description: "existing description", Version: 7}
				deleted := false
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					mu.Lock()
					defer mu.Unlock()
					switch {
					case req.Method == http.MethodGet && req.URL.Path == "/mint/api/vaults":
						_, _ = fmt.Fprint(w, `{"vaults":[{"id":"vault-123","name":"existing-vault"}]}`)
					case req.Method == http.MethodGet && req.URL.Path == "/mint/api/vaults/secrets/existing-secret" && req.URL.Query().Get("vault_id") == "vault-123":
						metadata := secret
						metadata.SecretValue = ""
						_ = json.NewEncoder(w).Encode(metadata)
					case req.Method == http.MethodPost && req.URL.Path == "/mint/api/vaults/secrets/existing-secret/terraform_management" && req.URL.Query().Get("vault_id") == "vault-123":
						w.WriteHeader(http.StatusNoContent)
					case req.Method == http.MethodPost && req.URL.Path == "/mint/api/vaults/secrets":
						var body struct {
							VaultID string       `json:"vault_id"`
							Secrets []api.Secret `json:"secrets"`
						}
						if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.VaultID != "vault-123" || len(body.Secrets) != 1 || body.Secrets[0].Name != secret.Name {
							http.Error(w, "invalid secret write", http.StatusBadRequest)
							return
						}
						version := secret.Version + 1
						secret = body.Secrets[0]
						secret.Version = version
						_ = json.NewEncoder(w).Encode(map[string]any{"versions": map[string]int{secret.Name: version}})
					case req.Method == http.MethodDelete && req.URL.Path == "/mint/api/vaults/secrets/existing-secret":
						deleted = true
					default:
						http.Error(w, "unexpected request", http.StatusBadRequest)
					}
				}))
				defer server.Close()
				previousClient := http.DefaultClient
				http.DefaultClient = server.Client()
				defer func() { http.DefaultClient = previousClient }()

				selectorValue := "existing-vault"
				importID := "existing-vault/existing-secret"
				if selector == "vault_id" {
					selectorValue = "vault-123"
					importID = "vault_id/vault-123/existing-secret"
				}
				configuredValue := "configured-value"
				if selector == "vault_id" && workflow == "block" {
					configuredValue = "original"
				}
				config := fmt.Sprintf(`
provider "rwx" {
  host = %q
  access_token = "test-token"
}
resource "rwx_secret" "test" {
  %s = %q
  name = "existing-secret"
  secret_value = %q
  description = "existing description"
}
`, strings.TrimPrefix(server.URL, "https://"), selector, selectorValue, configuredValue)
				checkRemote := func(version int, value string) error {
					mu.Lock()
					defer mu.Unlock()
					if deleted || secret.Version != version || secret.SecretValue != value {
						return fmt.Errorf("secret deleted=%t, version=%d, value=%q; want false, %d, %q", deleted, secret.Version, secret.SecretValue, version, value)
					}
					return nil
				}
				var steps []resource.TestStep
				applyConfig := config
				if workflow == "command" {
					steps = append(steps,
						resource.TestStep{Config: config, ExpectError: regexp.MustCompile("Secret already exists in Vault")},
						resource.TestStep{
							ResourceName:       "rwx_secret.test",
							ImportState:        true,
							ImportStateId:      importID,
							ImportStatePersist: true,
							ImportStateCheck: func(states []*terraform.InstanceState) error {
								if len(states) != 1 || states[0].Attributes[selector] != selectorValue || states[0].Attributes["name"] != "existing-secret" || states[0].Attributes["secret_value"] != "" || states[0].Attributes["description"] != "existing description" {
									return fmt.Errorf("unexpected imported state: %v", states)
								}
								return checkRemote(7, "original")
							},
						},
					)
				} else {
					applyConfig += fmt.Sprintf("\nimport {\n  to = rwx_secret.test\n  id = %q\n}\n", importID)
				}
				steps = append(steps,
					resource.TestStep{
						Config: applyConfig,
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr("rwx_secret.test", "secret_value", configuredValue),
							func(*terraform.State) error { return checkRemote(8, configuredValue) },
						),
					},
					resource.TestStep{Config: config, PlanOnly: true},
				)
				resource.UnitTest(t, resource.TestCase{
					ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
					Steps:                    steps,
				})
			})
		}
	}
}

func TestSecretResourceImportInvalidID(t *testing.T) {
	for _, id := range []string{"", "secret-only", "/secret", "vault/", "vault_id//secret", "vault_id/id/", "other/id/secret", "vault_id/id/secret/extra"} {
		t.Run(id, func(t *testing.T) {
			var resp frameworkresource.ImportStateResponse
			r := NewSecretResource().(*SecretResource)
			r.ImportState(context.Background(), frameworkresource.ImportStateRequest{ID: id}, &resp)
			if !resp.Diagnostics.HasError() {
				t.Fatal("expected invalid import ID diagnostic")
			}
		})
	}
}
