package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccessGrantResourceLifecycle(t *testing.T) {
	backend := newAccessGrantBackend()
	previousClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: backend}
	t.Cleanup(func() { http.DefaultClient = previousClient })

	var userGrantID string
	var serviceAccountGrantID string
	initialConfig := accessGrantConfig("user@example.com", "deploys", stringPointerValue("2099-12-01T15:00:00.000000Z"), nil)
	updatedConfig := accessGrantConfig("user@example.com", "deploys", stringPointerValue("2099-12-02T10:00:00-05:00"), stringPointerValue("2099-12-03T15:00:00Z"))

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: initialConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rwx_vault_user_access_grant.test", "email", "user@example.com"),
					resource.TestCheckResourceAttr("rwx_vault_user_access_grant.test", "expires_at", "2099-12-01T15:00:00.000000Z"),
					resource.TestCheckNoResourceAttr("rwx_vault_service_account_access_grant.test", "expires_at"),
					captureAccessGrantIDs(&userGrantID, &serviceAccountGrantID),
					checkAccessGrantCount(backend, 2),
				),
			},
			{
				Config:   initialConfig,
				PlanOnly: true,
			},
			{
				ResourceName:            "rwx_vault_user_access_grant.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"vault", "vault_id"},
			},
			{
				ResourceName:            "rwx_vault_service_account_access_grant.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"vault", "vault_id"},
			},
			{
				Config: updatedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rwx_vault_user_access_grant.test", "expires_at", "2099-12-02T10:00:00-05:00"),
					resource.TestCheckResourceAttr("rwx_vault_service_account_access_grant.test", "expires_at", "2099-12-03T15:00:00Z"),
					checkAccessGrantIDsUnchanged(&userGrantID, &serviceAccountGrantID),
				),
			},
			{
				Config:   updatedConfig,
				PlanOnly: true,
			},
			{
				PreConfig: func() {
					backend.setExpiration(userGrantID, stringPointerValue("2099-12-04T15:00:00Z"))
					backend.setExpiration(serviceAccountGrantID, nil)
				},
				Config:             updatedConfig,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: updatedConfig,
			},
			{
				PreConfig: func() {
					backend.delete(userGrantID)
					backend.delete(serviceAccountGrantID)
				},
				Config: updatedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAccessGrantIDsChanged(&userGrantID, &serviceAccountGrantID),
					checkAccessGrantCount(backend, 2),
				),
			},
			{
				Config: accessGrantConfig("other@example.com", "releases", stringPointerValue("2099-12-02T15:00:00Z"), nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkAccessGrantIDsChanged(&userGrantID, &serviceAccountGrantID),
					checkAccessGrantCount(backend, 2),
				),
			},
		},
	})
}

func TestAccessGrantResourceDoesNotAdoptExistingGrant(t *testing.T) {
	backend := newAccessGrantBackend()
	backend.grants["existing-grant"] = api.AccessGrant{
		ID:    "existing-grant",
		Vault: api.Vault{ID: "vault-id", Name: "test"},
		Principal: api.AccessGrantPrincipal{
			Type:  api.AccessGrantPrincipalTypeUser,
			ID:    "user-id",
			Email: "user@example.com",
		},
	}
	previousClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: backend}
	t.Cleanup(func() { http.DefaultClient = previousClient })

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "rwx" {
  host         = "access-grants.test"
  access_token = "test"
}

resource "rwx_vault_user_access_grant" "test" {
  vault_id = "vault-id"
  email    = "USER@example.com"
}
`,
				ExpectError: regexp.MustCompile(`Access grant already exists`),
			},
		},
	})

	if got := backend.count(); got != 1 {
		t.Fatalf("access grant count = %d, want existing grant only", got)
	}
}

func accessGrantConfig(email string, serviceAccount string, userExpiresAt *string, serviceAccountExpiresAt *string) string {
	return fmt.Sprintf(`
provider "rwx" {
  host         = "access-grants.test"
  access_token = "test"
}

resource "rwx_vault_user_access_grant" "test" {
  vault_id  = "vault-id"
  email     = %q
  %s
}

resource "rwx_vault_service_account_access_grant" "test" {
  vault_id       = "vault-id"
  service_account = %q
  %s
}
`, email, expirationConfig(userExpiresAt), serviceAccount, expirationConfig(serviceAccountExpiresAt))
}

func expirationConfig(expiresAt *string) string {
	if expiresAt == nil {
		return ""
	}
	return fmt.Sprintf("expires_at = %q", *expiresAt)
}

func captureAccessGrantIDs(userGrantID *string, serviceAccountGrantID *string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		*userGrantID = state.RootModule().Resources["rwx_vault_user_access_grant.test"].Primary.Attributes["id"]
		*serviceAccountGrantID = state.RootModule().Resources["rwx_vault_service_account_access_grant.test"].Primary.Attributes["id"]
		if *userGrantID == "" || *serviceAccountGrantID == "" {
			return fmt.Errorf("access grant IDs were not populated")
		}
		return nil
	}
}

func checkAccessGrantIDsChanged(userGrantID *string, serviceAccountGrantID *string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		newUserGrantID := state.RootModule().Resources["rwx_vault_user_access_grant.test"].Primary.Attributes["id"]
		newServiceAccountGrantID := state.RootModule().Resources["rwx_vault_service_account_access_grant.test"].Primary.Attributes["id"]
		if newUserGrantID == *userGrantID || newServiceAccountGrantID == *serviceAccountGrantID {
			return fmt.Errorf("access grant replacement preserved an old ID")
		}
		*userGrantID = newUserGrantID
		*serviceAccountGrantID = newServiceAccountGrantID
		return nil
	}
}

func checkAccessGrantIDsUnchanged(userGrantID *string, serviceAccountGrantID *string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		newUserGrantID := state.RootModule().Resources["rwx_vault_user_access_grant.test"].Primary.Attributes["id"]
		newServiceAccountGrantID := state.RootModule().Resources["rwx_vault_service_account_access_grant.test"].Primary.Attributes["id"]
		if newUserGrantID != *userGrantID || newServiceAccountGrantID != *serviceAccountGrantID {
			return fmt.Errorf("expiration update replaced an access grant")
		}
		return nil
	}
}

func checkAccessGrantCount(backend *accessGrantBackend, want int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if got := backend.count(); got != want {
			return fmt.Errorf("access grant count = %d, want %d", got, want)
		}
		return nil
	}
}

func stringPointerValue(value string) *string {
	return &value
}

type accessGrantBackend struct {
	mu     sync.Mutex
	nextID int
	grants map[string]api.AccessGrant
}

func newAccessGrantBackend() *accessGrantBackend {
	return &accessGrantBackend{grants: make(map[string]api.AccessGrant)}
}

func (b *accessGrantBackend) RoundTrip(req *http.Request) (*http.Response, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	path := req.URL.Path
	if req.Method == http.MethodGet && path == "/mint/api/vaults" {
		return accessGrantJSONResponse(http.StatusOK, map[string]any{"vaults": []api.Vault{{ID: "vault-id", Name: "test"}}}), nil
	}
	if path == "/mint/api/vaults/vault-id/access_grants" {
		switch req.Method {
		case http.MethodGet:
			grants := make([]api.AccessGrant, 0, len(b.grants))
			for _, grant := range b.grants {
				grants = append(grants, grant)
			}
			return accessGrantJSONResponse(http.StatusOK, map[string]any{"access_grants": grants}), nil
		case http.MethodPost:
			return b.create(req)
		}
	}

	const grantPath = "/mint/api/vaults/vault-id/access_grants/"
	if strings.HasPrefix(path, grantPath) {
		grantID := strings.TrimPrefix(path, grantPath)
		grant, ok := b.grants[grantID]
		switch req.Method {
		case http.MethodGet:
			if !ok {
				return accessGrantJSONResponse(http.StatusNotFound, nil), nil
			}
			return accessGrantJSONResponse(http.StatusOK, map[string]any{"access_grant": grant}), nil
		case http.MethodPatch:
			if !ok {
				return accessGrantJSONResponse(http.StatusNotFound, nil), nil
			}
			var body struct {
				ExpiresAt *string `json:"expires_at"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				return nil, err
			}
			grant.ExpiresAt = normalizedExpiration(body.ExpiresAt)
			b.grants[grantID] = grant
			return accessGrantJSONResponse(http.StatusOK, map[string]any{"access_grant": grant}), nil
		case http.MethodDelete:
			delete(b.grants, grantID)
			return accessGrantJSONResponse(http.StatusNoContent, nil), nil
		}
	}

	return accessGrantJSONResponse(http.StatusNotFound, nil), nil
}

func (b *accessGrantBackend) create(req *http.Request) (*http.Response, error) {
	var body struct {
		PrincipalType  string  `json:"principal_type"`
		Email          string  `json:"email"`
		ServiceAccount string  `json:"service_account"`
		ExpiresAt      *string `json:"expires_at"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return nil, err
	}

	for _, grant := range b.grants {
		if grant.Principal.Type == body.PrincipalType && (grant.Principal.Email == body.Email || grant.Principal.Name == body.ServiceAccount) {
			return accessGrantJSONResponse(http.StatusCreated, map[string]any{"access_grant": grant}), nil
		}
	}

	b.nextID++
	grantID := fmt.Sprintf("grant-%d", b.nextID)
	principal := api.AccessGrantPrincipal{Type: body.PrincipalType, ID: "principal-" + grantID, Email: body.Email, Name: body.ServiceAccount}
	grant := api.AccessGrant{
		ID:        grantID,
		Vault:     api.Vault{ID: "vault-id", Name: "test"},
		Principal: principal,
		ExpiresAt: normalizedExpiration(body.ExpiresAt),
	}
	b.grants[grantID] = grant
	return accessGrantJSONResponse(http.StatusCreated, map[string]any{"access_grant": grant}), nil
}

func (b *accessGrantBackend) setExpiration(grantID string, expiresAt *string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	grant := b.grants[grantID]
	grant.ExpiresAt = normalizedExpiration(expiresAt)
	b.grants[grantID] = grant
}

func (b *accessGrantBackend) delete(grantID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.grants, grantID)
}

func (b *accessGrantBackend) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.grants)
}

func normalizedExpiration(expiresAt *string) *string {
	if expiresAt == nil {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, *expiresAt)
	if err != nil {
		return expiresAt
	}
	normalized := parsed.UTC().Format("2006-01-02T15:04:05.000000Z")
	return &normalized
}

func accessGrantJSONResponse(status int, body any) *http.Response {
	var encoded []byte
	if body != nil {
		encoded, _ = json.Marshal(body)
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(strings.NewReader(string(encoded))),
	}
}
