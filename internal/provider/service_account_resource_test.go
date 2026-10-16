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

type serviceAccountBackend struct {
	mu           sync.Mutex
	account      *api.ServiceAccount
	nextID       int
	status       int
	statusMethod string
	lookupCount  int
	secondPage   bool
}

func (b *serviceAccountBackend) RoundTrip(req *http.Request) (*http.Response, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if req.Header.Get("Authorization") != "Bearer service-account-test-token" {
		return nil, fmt.Errorf("missing bearer authentication")
	}
	status := http.StatusOK
	var result any
	if b.status != 0 && (b.statusMethod == "" || b.statusMethod == req.Method) {
		status = b.status
		result = map[string]any{"errors": []string{"guarded failure"}}
	} else if req.Method == http.MethodGet && req.URL.Path == "/mint/api/service_accounts" {
		matches := []api.ServiceAccount{}
		var cursor *string
		if b.secondPage && req.URL.Query().Get("cursor") == "" {
			value := "next+/=&"
			cursor = &value
			if b.lookupCount > 1 {
				matches = append(matches, *b.account)
			}
		} else {
			if b.secondPage && req.URL.Query().Get("cursor") != "next+/=&" {
				return nil, fmt.Errorf("cursor not preserved")
			}
			if req.URL.Query().Get("name") == "" && req.URL.Query().Get("display_name") == "" {
				return nil, fmt.Errorf("filter not preserved")
			}
			for i := 0; i < b.lookupCount; i++ {
				if b.secondPage && b.lookupCount > 1 && i == 0 {
					continue
				}
				matches = append(matches, *b.account)
			}
		}
		result = map[string]any{"service_accounts": matches, "pagination": map[string]any{"next_cursor": cursor, "limit": 100}}
	} else {
		switch req.Method {
		case http.MethodPost, http.MethodPatch:
			var body map[string]json.RawMessage
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				return nil, err
			}
			if len(body) != 2 || body["display_name"] == nil || body["description"] == nil {
				return nil, fmt.Errorf("metadata request contains unexpected fields: %v", body)
			}
			var displayName string
			var description *string
			if err := json.Unmarshal(body["display_name"], &displayName); err != nil {
				return nil, err
			}
			if err := json.Unmarshal(body["description"], &description); err != nil {
				return nil, err
			}
			if req.Method == http.MethodPost {
				if b.account != nil {
					status = http.StatusConflict
					result = map[string]any{"errors": []string{"display name already exists"}}
					break
				}
				b.nextID++
				b.account = &api.ServiceAccount{ID: fmt.Sprintf("account-%d", b.nextID), IdentityID: fmt.Sprintf("identity-%d", b.nextID), Name: fmt.Sprintf("immutable-%d", b.nextID)}
				status = http.StatusCreated
			} else if b.account == nil {
				status = http.StatusNotFound
				break
			}
			b.account.DisplayName = displayName
			b.account.Description = description
			result = map[string]any{"service_account": b.account}
		case http.MethodGet:
			if b.account == nil || !strings.HasSuffix(req.URL.Path, "/"+b.account.ID) {
				status = http.StatusNotFound
			} else {
				result = map[string]any{"service_account": b.account}
			}
		case http.MethodDelete:
			if b.account == nil {
				status = http.StatusNotFound
			} else {
				b.account = nil
				status = http.StatusNoContent
			}
		default:
			return nil, fmt.Errorf("unexpected request: %s %s", req.Method, req.URL)
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: status, Status: fmt.Sprintf("%d %s", status, http.StatusText(status)), Body: io.NopCloser(strings.NewReader(string(encoded))), Header: http.Header{}}, nil
}

func useServiceAccountBackend(t *testing.T, backend *serviceAccountBackend) {
	previous := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: backend}
	t.Cleanup(func() { http.DefaultClient = previous })
}

const serviceAccountProviderConfig = `
provider "rwx" {
  host = "service-accounts.test"
  access_token = "service-account-test-token"
}
`

func serviceAccountConfig(displayName, description string) string {
	return serviceAccountProviderConfig + fmt.Sprintf(`
resource "rwx_service_account" "test" {
  display_name = %q
  description = %s
}
`, displayName, description)
}

func TestServiceAccountLifecycle(t *testing.T) {
	backend := &serviceAccountBackend{}
	useServiceAccountBackend(t, backend)
	initial := serviceAccountConfig("Deploy automation", `"Initial description"`)
	renamed := serviceAccountConfig("Release automation", "null")
	var id string
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: initial, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("rwx_service_account.test", "name", "immutable-1"),
				resource.TestCheckResourceAttr("rwx_service_account.test", "identity_id", "identity-1"),
				resource.TestCheckResourceAttr("rwx_service_account.test", "description", "Initial description"),
				func(s *terraform.State) error {
					id = s.RootModule().Resources["rwx_service_account.test"].Primary.ID
					return nil
				},
			)},
			{Config: initial, PlanOnly: true},
			{ResourceName: "rwx_service_account.test", ImportState: true, ImportStateVerify: true},
			{Config: initial, PlanOnly: true},
			{Config: renamed, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("rwx_service_account.test", "display_name", "Release automation"),
				resource.TestCheckNoResourceAttr("rwx_service_account.test", "description"),
				resource.TestCheckResourceAttr("rwx_service_account.test", "name", "immutable-1"),
				resource.TestCheckResourceAttr("rwx_service_account.test", "identity_id", "identity-1"),
				func(s *terraform.State) error {
					if s.RootModule().Resources["rwx_service_account.test"].Primary.ID != id {
						return fmt.Errorf("rename changed ID")
					}
					return nil
				},
			)},
			{Config: renamed, PlanOnly: true},
			{PreConfig: func() {
				backend.mu.Lock()
				defer backend.mu.Unlock()
				backend.account.DisplayName = "Remote drift"
				description := "Remote description"
				backend.account.Description = &description
			}, Config: renamed, PlanOnly: true, ExpectNonEmptyPlan: true},
			{Config: renamed},
			{Config: renamed, PlanOnly: true},
			{PreConfig: func() { backend.mu.Lock(); defer backend.mu.Unlock(); backend.account = nil }, Config: renamed, Check: resource.TestCheckResourceAttr("rwx_service_account.test", "id", "account-2")},
		},
	})
	if backend.account != nil {
		t.Fatal("destroy did not deactivate account")
	}
}

func TestServiceAccountLookup(t *testing.T) {
	for _, selector := range []string{"id", "name", "display_name"} {
		t.Run(selector, func(t *testing.T) {
			backend := &serviceAccountBackend{account: &api.ServiceAccount{ID: "account-1", IdentityID: "identity-1", Name: "immutable-1", DisplayName: "Deploy automation"}, lookupCount: 1, secondPage: true}
			useServiceAccountBackend(t, backend)
			value := map[string]string{"id": "account-1", "name": "immutable-1", "display_name": "Deploy automation"}[selector]
			resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
				{Config: serviceAccountProviderConfig + fmt.Sprintf(`data "rwx_service_account" "test" { %s = %q }`, selector, value), Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.rwx_service_account.test", "id", "account-1"),
					resource.TestCheckResourceAttr("data.rwx_service_account.test", "name", "immutable-1"),
					resource.TestCheckResourceAttr("data.rwx_service_account.test", "display_name", "Deploy automation"),
				)},
			}})
		})
	}
}

func TestServiceAccountLookupPlanErrors(t *testing.T) {
	for _, test := range []struct {
		name, selector, pattern string
		count, status           int
	}{
		{"missing-name", `name="missing"`, "not found", 0, 0},
		{"missing-display-name", `display_name="missing"`, "not found", 0, 0},
		{"missing-id", `id="missing"`, "not found", 1, 0},
		{"ambiguous", `display_name="duplicate"`, "multiple active service accounts matched", 2, 0},
		{"unauthorized", `name="immutable-1"`, "guarded failure", 1, 401},
		{"forbidden", `id="account-1"`, "guarded failure", 1, 403},
		{"no-selector", "", "Invalid Attribute Combination", 0, 0},
		{"conflicting-selectors", "name=\"one\"\nid=\"two\"", "Invalid Attribute Combination", 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			useServiceAccountBackend(t, &serviceAccountBackend{account: &api.ServiceAccount{ID: "account-1"}, lookupCount: test.count, secondPage: true, status: test.status})
			resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{{Config: serviceAccountProviderConfig + "data \"rwx_service_account\" \"test\" {\n" + test.selector + "\n}", PlanOnly: true, ExpectError: regexp.MustCompile(test.pattern)}}})
		})
	}
}

func TestServiceAccountLookupUnknownID(t *testing.T) {
	useServiceAccountBackend(t, &serviceAccountBackend{})
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{{Config: serviceAccountConfig("Deploy", "null") + `
data "rwx_service_account" "test" { id = rwx_service_account.test.id }
`, Check: resource.TestCheckResourceAttr("data.rwx_service_account.test", "name", "immutable-1")}}})
}

func TestServiceAccountCreateConflict(t *testing.T) {
	useServiceAccountBackend(t, &serviceAccountBackend{account: &api.ServiceAccount{ID: "existing"}})
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{{Config: serviceAccountConfig("Existing", "null"), ExpectError: regexp.MustCompile("display name already exists")}}})
}

func TestServiceAccountGuardedFailures(t *testing.T) {
	for _, test := range []struct {
		method string
		status int
	}{
		{http.MethodGet, 401}, {http.MethodGet, 403}, {http.MethodPatch, 409}, {http.MethodDelete, 403}, {http.MethodDelete, 422},
	} {
		t.Run(fmt.Sprintf("%s-%d", test.method, test.status), func(t *testing.T) {
			backend := &serviceAccountBackend{}
			useServiceAccountBackend(t, backend)
			config := serviceAccountConfig("Deploy", "null")
			failure := resource.TestStep{
				PreConfig: func() {
					backend.mu.Lock()
					defer backend.mu.Unlock()
					backend.status = test.status
					backend.statusMethod = test.method
				},
				Config: config, ExpectError: regexp.MustCompile("guarded failure"),
			}
			if test.method == http.MethodGet {
				failure.PlanOnly = true
			}
			if test.method == http.MethodPatch {
				failure.Config = serviceAccountConfig("Rename", "null")
			}
			if test.method == http.MethodDelete {
				failure.Destroy = true
			}
			resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
				{Config: config}, failure,
				{PreConfig: func() { backend.mu.Lock(); defer backend.mu.Unlock(); backend.status = 0 }, Config: config, PlanOnly: true, Check: resource.TestCheckResourceAttr("rwx_service_account.test", "id", "account-1")},
			}})
		})
	}
}
