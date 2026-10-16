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

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type serviceAccountPermissionsBackend struct {
	mu          sync.Mutex
	direct      map[string][]api.Permission
	token       string
	readStatus  int
	writeStatus int
	writes      int
}

func newServiceAccountPermissionsBackend(t *testing.T, token string) *serviceAccountPermissionsBackend {
	t.Helper()
	backend := &serviceAccountPermissionsBackend{direct: map[string][]api.Permission{"account-one": {}, "account-two": {}}, token: token}
	previous := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: backend}
	t.Cleanup(func() { http.DefaultClient = previous })
	return backend
}

func (b *serviceAccountPermissionsBackend) RoundTrip(req *http.Request) (*http.Response, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	respond := func(status int, body any) (*http.Response, error) {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: status, Status: fmt.Sprintf("%d %s", status, http.StatusText(status)), Body: io.NopCloser(strings.NewReader(string(encoded))), Header: http.Header{"Content-Type": {"application/json"}}}, nil
	}
	if req.Header.Get("Authorization") != "Bearer "+b.token {
		return respond(401, map[string]string{"error": "Unauthorized"})
	}
	if b.readStatus != 0 && req.Method == http.MethodGet {
		return respond(b.readStatus, map[string][]string{"errors": {"fixture read denied"}})
	}
	if req.URL.Path == "/mint/api/permission_catalog" && req.Method == http.MethodGet {
		id := req.URL.Query().Get("service_account_id")
		if _, ok := b.direct[id]; id != "" && !ok {
			return respond(404, nil)
		}
		var catalog api.PermissionCatalog
		_ = json.Unmarshal([]byte(`{"permissions":[{"key":"run:read","supports_resource_scope":false,"grantable":{"unscoped":true,"resource_ids":[]}},{"key":"vault:manage","supports_resource_scope":true,"scope_resource":"vault","grantable":{"unscoped":true,"resource_ids":["vault-one"]}},{"key":"cron:manage","supports_resource_scope":false,"grantable":{"unscoped":false,"resource_ids":[]}}],"unremovable_permissions":[]}`), &catalog)
		for _, permission := range b.direct[id] {
			if permission.Key == "cron:manage" {
				catalog.UnremovablePermissions = append(catalog.UnremovablePermissions, permission.Key)
			}
		}
		return respond(200, catalog)
	}
	if strings.HasPrefix(req.URL.Path, "/mint/api/service_accounts/") && strings.HasSuffix(req.URL.Path, "/permissions") {
		id := strings.TrimSuffix(strings.TrimPrefix(req.URL.Path, "/mint/api/service_accounts/"), "/permissions")
		permissions, ok := b.direct[id]
		if !ok {
			return respond(404, nil)
		}
		if req.Method == http.MethodPut {
			if b.writeStatus != 0 {
				return respond(b.writeStatus, map[string][]string{"errors": {"fixture replacement denied"}})
			}
			var body api.Permissions
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Permissions == nil {
				return respond(422, map[string][]string{"errors": {"permissions must be an array"}})
			}
			permissions = body.Permissions
			b.direct[id] = permissions
			b.writes++
		} else if req.Method != http.MethodGet {
			return respond(405, nil)
		}
		return respond(200, api.Permissions{Permissions: permissions})
	}
	return nil, fmt.Errorf("unexpected fixture request: %s %s", req.Method, req.URL)
}

func serviceAccountPermissionsConfig(token, id, permissions string) string {
	return fmt.Sprintf(`provider "rwx" { access_token = %q }
resource "rwx_service_account_permissions" "test" {
  service_account_id = %q
  permissions = %s
}`, token, id, permissions)
}

func TestServiceAccountPermissionsLifecycle(t *testing.T) {
	for _, token := range []string{"fixture-pat", "fixture-service-account"} {
		t.Run(token, func(t *testing.T) {
			b := newServiceAccountPermissionsBackend(t, token)
			initial := serviceAccountPermissionsConfig(token, "account-one", `[{key="run:read"},{key="vault:manage",resource_id="vault-one"}]`)
			cleared := serviceAccountPermissionsConfig(token, "account-one", `[]`)
			replaced := serviceAccountPermissionsConfig(token, "account-two", `[{key="run:read"}]`)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy: func(_ *terraform.State) error {
					b.mu.Lock()
					defer b.mu.Unlock()
					for id, permissions := range b.direct {
						if len(permissions) != 0 {
							return fmt.Errorf("%s still has direct permissions", id)
						}
					}
					return nil
				},
				Steps: []resource.TestStep{
					{Config: initial, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("rwx_service_account_permissions.test", "id", "account-one"), resource.TestCheckResourceAttr("rwx_service_account_permissions.test", "permissions.#", "2"))},
					{Config: initial, PlanOnly: true},
					{ResourceName: "rwx_service_account_permissions.test", ImportState: true, ImportStateVerify: true},
					{Config: initial, PlanOnly: true},
					{PreConfig: func() { b.mu.Lock(); b.direct["account-one"] = []api.Permission{{Key: "run:read"}}; b.mu.Unlock() }, Config: initial, PlanOnly: true, ExpectNonEmptyPlan: true},
					{Config: initial},
					{Config: initial, PlanOnly: true},
					{Config: cleared, Check: resource.TestCheckResourceAttr("rwx_service_account_permissions.test", "permissions.#", "0")},
					{Config: initial},
					{Config: replaced, Check: resource.TestCheckResourceAttr("rwx_service_account_permissions.test", "id", "account-two")},
					{PreConfig: func() { b.mu.Lock(); delete(b.direct, "account-two"); b.mu.Unlock() }, Config: fmt.Sprintf(`provider "rwx" { access_token = %q }`, token), Check: func(state *terraform.State) error {
						if _, ok := state.RootModule().Resources["rwx_service_account_permissions.test"]; ok {
							return fmt.Errorf("missing target retained in state")
						}
						return nil
					}},
				},
			})
		})
	}
}

func TestServiceAccountPermissionsPlanRejections(t *testing.T) {
	for _, test := range []struct{ name, permissions, message string }{
		{"unknown key", `[{key="run:read"},{key="unknown:write"}]`, "Invalid permission key"},
		{"unsupported scope", `[{key="run:read",resource_id="vault-one"}]`, "Invalid permission scope"},
		{"ungrantable", `[{key="run:read"},{key="cron:manage"}]`, "Permission cannot be granted"},
		{"unlisted scoped ID despite unscoped authority", `[{key="vault:manage",resource_id="foreign-vault"}]`, "Permission cannot be granted"},
		{"blank scope", `[{key="vault:manage",resource_id=" "}]`, "must not be blank"},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := newServiceAccountPermissionsBackend(t, "fixture-service-account")
			resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{{Config: serviceAccountPermissionsConfig(b.token, "account-one", test.permissions), PlanOnly: true, ExpectError: regexp.MustCompile(test.message)}}})
			if b.writes != 0 {
				t.Fatalf("invalid plan made %d writes", b.writes)
			}
		})
	}
}

func TestServiceAccountPermissionsRetainedAndUnremovable(t *testing.T) {
	b := newServiceAccountPermissionsBackend(t, "fixture-service-account")
	b.direct["account-one"] = []api.Permission{{Key: "cron:manage"}}
	retained := serviceAccountPermissionsConfig(b.token, "account-one", `[{key="cron:manage"}]`)
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
		{Config: retained, PlanOnly: true, ExpectNonEmptyPlan: true},
		{Config: serviceAccountPermissionsConfig(b.token, "account-one", `[]`), PlanOnly: true, ExpectError: regexp.MustCompile("Permission cannot be removed")},
	}})
	if b.writes != 0 {
		t.Fatalf("plan made %d writes", b.writes)
	}
}

func TestServiceAccountPermissionsFailures(t *testing.T) {
	for _, status := range []int{401, 403, 422} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			b := newServiceAccountPermissionsBackend(t, "fixture-service-account")
			config := serviceAccountPermissionsConfig(b.token, "account-one", `[{key="run:read"}]`)
			resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
				{Config: config},
				{PreConfig: func() { b.mu.Lock(); b.readStatus = status; b.mu.Unlock() }, Config: config, PlanOnly: true, ExpectError: regexp.MustCompile("fixture read denied")},
				{PreConfig: func() { b.mu.Lock(); b.readStatus = 0; b.writeStatus = status; b.mu.Unlock() }, Config: serviceAccountPermissionsConfig(b.token, "account-one", `[]`), ExpectError: regexp.MustCompile("fixture replacement denied")},
				{PreConfig: func() { b.mu.Lock(); b.writeStatus = 0; b.mu.Unlock() }, Config: config, PlanOnly: true},
				{PreConfig: func() { b.mu.Lock(); b.writeStatus = status; b.mu.Unlock() }, Config: config, Destroy: true, ExpectError: regexp.MustCompile("fixture replacement denied")},
				{PreConfig: func() { b.mu.Lock(); b.writeStatus = 0; b.mu.Unlock() }, Config: config, PlanOnly: true},
			}})
		})
	}
}

func TestServiceAccountPermissionsUnknownIDs(t *testing.T) {
	b := newServiceAccountPermissionsBackend(t, "fixture-service-account")
	config := `provider "rwx" { access_token = "fixture-service-account" }
resource "terraform_data" "account" { input = "account-one" }
resource "terraform_data" "vault" { input = "vault-one" }
resource "rwx_service_account_permissions" "test" {
  service_account_id = terraform_data.account.output
  permissions = [{key="vault:manage", resource_id=terraform_data.vault.output}]
}`
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
		{Config: config, PlanOnly: true, ExpectNonEmptyPlan: true},
		{Config: config, Check: resource.TestCheckResourceAttr("rwx_service_account_permissions.test", "permissions.#", "1")},
		{Config: config, PlanOnly: true},
	}})
	if b.writes == 0 {
		t.Fatal("unknown same-plan IDs never applied")
	}
}
