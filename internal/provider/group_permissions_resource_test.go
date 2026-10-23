package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"
)

func TestGroupPermissionsResourceLifecycle(t *testing.T) {
	b := installGroupPermissionsBackend(t)
	initial := groupPermissionsConfig("group-one", `[{ key = "run:read" }, { key = "vault:unlock", resource_id = "vault-one" }]`)
	cleared := groupPermissionsConfig("group-one", `[]`)
	replaced := groupPermissionsConfig("group-two", `[{ key = "run:read" }]`)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: initial, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("rwx_group_permissions.test", "id", "group-one"),
				resource.TestCheckResourceAttr("rwx_group_permissions.test", "permissions.#", "2"),
				b.check("group-one", []api.Permission{{Key: "run:read"}, {Key: "vault:unlock", ResourceID: stringPointerValue("vault-one")}}),
			)},
			{Config: initial, PlanOnly: true},
			{ResourceName: "rwx_group_permissions.test", ImportState: true, ImportStateVerify: true},
			{Config: initial, PlanOnly: true},
			{PreConfig: func() { b.setPermissions("group-one", []api.Permission{{Key: "run:read"}}) }, Config: initial, PlanOnly: true, ExpectNonEmptyPlan: true},
			{Config: initial, Check: b.check("group-one", []api.Permission{{Key: "run:read"}, {Key: "vault:unlock", ResourceID: stringPointerValue("vault-one")}})},
			{Config: initial, PlanOnly: true},
			{PreConfig: func() { b.setFailure(http.MethodGet, http.StatusForbidden) }, Config: initial, PlanOnly: true, ExpectError: regexp.MustCompile("group:manage")},
			{PreConfig: func() { b.setFailure("", 0) }, Config: initial, PlanOnly: true},
			{PreConfig: func() { b.setFailure(http.MethodPut, http.StatusUnprocessableEntity) }, Config: cleared, ExpectError: regexp.MustCompile("authority changed")},
			{PreConfig: func() { b.setFailure("", 0) }, Config: initial, PlanOnly: true},
			{PreConfig: func() { b.setFailure(http.MethodPut, http.StatusForbidden) }, Config: cleared, Destroy: true, ExpectError: regexp.MustCompile("group:manage")},
			{PreConfig: func() { b.setFailure("", 0) }, Config: initial, PlanOnly: true},
			{Config: cleared, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("rwx_group_permissions.test", "permissions.#", "0"),
				b.check("group-one", []api.Permission{}),
			)},
			{Config: cleared, PlanOnly: true},
			{Config: initial},
			{Config: replaced, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("rwx_group_permissions.test", "id", "group-two"),
				b.check("group-one", []api.Permission{}),
				b.check("group-two", []api.Permission{{Key: "run:read"}}),
			)},
			{Config: replaced, PlanOnly: true},
		},
	})
	if err := b.check("group-two", []api.Permission{})(nil); err != nil {
		t.Fatal(err)
	}
}

func TestGroupPermissionsResourcePlanRejections(t *testing.T) {
	for _, tc := range []struct{ name, permissions, message string }{
		{"unknown key", `[{ key = "run:read" }, { key = "not:a:permission" }]`, "Invalid permission key"},
		{"unsupported scope", `[{ key = "run:read", resource_id = "vault-one" }]`, "Invalid permission scope"},
		{"foreign scope", `[{ key = "vault:unlock", resource_id = "foreign-vault" }]`, "Permission cannot be granted"},
		{"restricted token", `[{ key = "run:read" }, { key = "cron:manage" }]`, "Permission cannot be granted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := installGroupPermissionsBackend(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps:                    []resource.TestStep{{Config: groupPermissionsConfig("group-one", tc.permissions), PlanOnly: true, ExpectError: regexp.MustCompile(tc.message)}},
			})
			if err := b.check("group-one", []api.Permission{})(nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGroupPermissionsResourceRemoteDeletion(t *testing.T) {
	b := installGroupPermissionsBackend(t)
	config := groupPermissionsConfig("group-one", `[{ key = "run:read" }]`)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config},
			{PreConfig: func() { b.mu.Lock(); delete(b.direct, "group-one"); b.mu.Unlock() }, Config: `
provider "rwx" {
  host = "group-permissions.test"
  access_token = "service-account-test-token"
}
`, Check: func(s *terraform.State) error {
				if _, ok := s.RootModule().Resources["rwx_group_permissions.test"]; ok {
					return fmt.Errorf("deleted group permissions remained in state")
				}
				return nil
			}},
			{Config: config, PlanOnly: true, ExpectError: regexp.MustCompile("not found")},
		},
	})
}

func TestGroupPermissionsResourceExistingAuthority(t *testing.T) {
	b := installGroupPermissionsBackend(t)
	b.setPermissions("group-one", []api.Permission{{Key: "cron:manage"}})
	b.unremovable = []string{"cron:manage"}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: groupPermissionsConfig("group-one", `[{ key = "cron:manage" }]`), PlanOnly: true, ExpectNonEmptyPlan: true},
			{Config: groupPermissionsConfig("group-one", `[]`), PlanOnly: true, ExpectError: regexp.MustCompile("Permission cannot be removed")},
		},
	})
	if err := b.check("group-one", []api.Permission{{Key: "cron:manage"}})(nil); err != nil {
		t.Fatal(err)
	}
}

func TestGroupPermissionsResourceSamePlanIDs(t *testing.T) {
	b := installGroupPermissionsBackend(t)
	config := strings.ReplaceAll(groupPermissionsConfig("group-one", `[{ key = "vault:unlock", resource_id = terraform_data.scope.output }]`), `group_id = "group-one"`, `group_id = terraform_data.target.output`) + `
resource "terraform_data" "target" {
  input = "group-one"
}
resource "terraform_data" "scope" {
  input = "vault-one"
}
`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config, Check: b.check("group-one", []api.Permission{{Key: "vault:unlock", ResourceID: stringPointerValue("vault-one")}})},
			{Config: config, PlanOnly: true},
		},
	})
}

func groupPermissionsConfig(id, permissions string) string {
	return fmt.Sprintf(`
provider "rwx" {
  host = "group-permissions.test"
  access_token = "service-account-test-token"
}
resource "rwx_group_permissions" "test" {
  group_id = %q
  permissions = %s
}
`, id, permissions)
}

type groupPermissionsBackend struct {
	mu          sync.Mutex
	direct      map[string][]api.Permission
	inherited   map[string][]api.Permission
	unremovable []string
	failMethod  string
	failStatus  int
}

func installGroupPermissionsBackend(t *testing.T) *groupPermissionsBackend {
	t.Helper()
	b := &groupPermissionsBackend{
		direct:    map[string][]api.Permission{"group-one": {}, "group-two": {}},
		inherited: map[string][]api.Permission{"group-one": {{Key: "run:manage"}}, "group-two": {{Key: "run:manage"}}},
	}
	previous := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: b}
	t.Cleanup(func() { http.DefaultClient = previous })
	return b
}

func (b *groupPermissionsBackend) setPermissions(id string, permissions []api.Permission) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.direct[id] = permissions
}

func (b *groupPermissionsBackend) setFailure(method string, status int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failMethod, b.failStatus = method, status
}

func (b *groupPermissionsBackend) check(id string, want []api.Permission) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		b.mu.Lock()
		defer b.mu.Unlock()
		if !reflect.DeepEqual(b.direct[id], want) {
			return fmt.Errorf("direct permissions = %v, want %v", b.direct[id], want)
		}
		if !reflect.DeepEqual(b.inherited[id], []api.Permission{{Key: "run:manage"}}) {
			return fmt.Errorf("inherited access changed: %v", b.inherited[id])
		}
		return nil
	}
}

func (b *groupPermissionsBackend) RoundTrip(req *http.Request) (*http.Response, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if req.Header.Get("Authorization") != "Bearer service-account-test-token" {
		return groupPermissionsResponse(http.StatusUnauthorized, map[string]any{"error": "Unauthorized"}), nil
	}
	if req.Method == b.failMethod {
		if b.failStatus == http.StatusUnprocessableEntity {
			return groupPermissionsResponse(b.failStatus, map[string]any{"errors": []string{"Permission authority changed; refresh and retry"}}), nil
		}
		return groupPermissionsResponse(b.failStatus, map[string]any{"error_messages": []map[string]string{{"message": "Requires group:manage permission"}}}), nil
	}
	if req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/terraform_management") {
		return groupPermissionsResponse(http.StatusNoContent, nil), nil
	}
	if req.URL.Path == "/mint/api/permission_catalog" {
		id := req.URL.Query().Get("group_id")
		if _, ok := b.direct[id]; !ok && id != "" {
			return groupPermissionsResponse(http.StatusNotFound, nil), nil
		}
		return groupPermissionsResponse(http.StatusOK, map[string]any{
			"permissions": []any{
				map[string]any{"key": "run:read", "supports_resource_scope": false, "scope_resource": nil, "grantable": map[string]any{"unscoped": true, "resource_ids": []string{}}},
				map[string]any{"key": "vault:unlock", "supports_resource_scope": true, "scope_resource": "vault", "grantable": map[string]any{"unscoped": true, "resource_ids": []string{"vault-one"}}},
				map[string]any{"key": "cron:manage", "supports_resource_scope": false, "scope_resource": nil, "grantable": map[string]any{"unscoped": false, "resource_ids": []string{}}},
			},
			"unremovable_permissions": b.unremovable,
		}), nil
	}
	id := strings.TrimSuffix(strings.TrimPrefix(req.URL.Path, "/mint/api/groups/"), "/permissions")
	permissions, ok := b.direct[id]
	if !ok || req.URL.Path != "/mint/api/groups/"+id+"/permissions" {
		return groupPermissionsResponse(http.StatusNotFound, nil), nil
	}
	switch req.Method {
	case http.MethodGet:
		return groupPermissionsResponse(http.StatusOK, api.Permissions{Permissions: permissions}), nil
	case http.MethodPut:
		var body map[string]json.RawMessage
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			return nil, err
		}
		if len(body) != 1 || string(body["permissions"]) == "null" || req.Header.Get("Content-Type") != "application/json" {
			return nil, fmt.Errorf("expected JSON permissions array, got %v", body)
		}
		if err := json.Unmarshal(body["permissions"], &permissions); err != nil {
			return nil, err
		}
		b.direct[id] = permissions
		return groupPermissionsResponse(http.StatusOK, api.Permissions{Permissions: permissions}), nil
	}
	return nil, fmt.Errorf("unexpected request: %s %s", req.Method, req.URL)
}

func groupPermissionsResponse(status int, body any) *http.Response {
	encoded, _ := json.Marshal(body)
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(string(encoded)))}
}
