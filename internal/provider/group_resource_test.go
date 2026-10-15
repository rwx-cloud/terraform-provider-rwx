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

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"
)

func TestGroupResourceLifecycle(t *testing.T) {
	b := installGroupBackend(t)
	initial := groupTestConfig("Deployers", `description = "Deploy team"`)
	renamed := groupTestConfig("Release engineers", `description = "Release team"`)
	cleared := groupTestConfig("Release engineers", "")
	var id string
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: initial, Check: func(s *terraform.State) error {
				id = s.RootModule().Resources["rwx_group.test"].Primary.ID
				if id == "" {
					return fmt.Errorf("missing group ID")
				}
				return nil
			}},
			{Config: initial, PlanOnly: true},
			{ResourceName: "rwx_group.test", ImportState: true, ImportStateVerify: true},
			{Config: initial, PlanOnly: true},
			{Config: renamed, Check: resource.ComposeAggregateTestCheckFunc(
				checkResourceAttribute("rwx_group.test", "id", &id),
				resource.TestCheckResourceAttr("rwx_group.test", "name", "Release engineers"),
				resource.TestCheckResourceAttr("rwx_group.test", "description", "Release team"),
			)},
			{Config: cleared, Check: resource.TestCheckNoResourceAttr("rwx_group.test", "description")},
			{Config: cleared, PlanOnly: true},
			{PreConfig: func() {
				b.mu.Lock()
				defer b.mu.Unlock()
				g := b.groups[id]
				g.Name = "Remote rename"
				description := "Remote description"
				g.Description = &description
				b.groups[id] = g
			}, Config: cleared, PlanOnly: true, ExpectNonEmptyPlan: true},
			{Config: cleared, Check: checkResourceAttribute("rwx_group.test", "id", &id)},
			{Config: cleared, PlanOnly: true},
			{PreConfig: func() { b.setFailure(http.MethodGet, http.StatusForbidden) }, Config: cleared, PlanOnly: true, ExpectError: regexp.MustCompile("group:manage")},
			{PreConfig: func() { b.setFailure("", 0) }, Config: cleared, PlanOnly: true},
			{PreConfig: func() { b.setFailure(http.MethodPatch, http.StatusForbidden) }, Config: renamed, ExpectError: regexp.MustCompile("group:manage")},
			{PreConfig: func() { b.setFailure("", 0) }, Config: cleared, PlanOnly: true},
			{PreConfig: func() { b.setFailure(http.MethodDelete, http.StatusConflict) }, Config: cleared, Destroy: true, ExpectError: regexp.MustCompile("System groups cannot be edited or deleted")},
			{PreConfig: func() { b.setFailure("", 0) }, Config: cleared, PlanOnly: true},
			{PreConfig: func() { b.mu.Lock(); delete(b.groups, id); b.mu.Unlock() }, Config: cleared, Check: func(s *terraform.State) error {
				got := s.RootModule().Resources["rwx_group.test"].Primary.ID
				if got == id {
					return fmt.Errorf("remote deletion did not recreate group")
				}
				return nil
			}},
		},
	})
}

func TestGroupDataSourceSamePlan(t *testing.T) {
	installGroupBackend(t)
	config := groupTestConfig("Deployers", "") + `
data "rwx_group" "by_id" {
  id = rwx_group.test.id
}
`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config, Check: resource.TestCheckResourceAttr("data.rwx_group.by_id", "name", "Deployers")},
			{Config: config, PlanOnly: true},
		},
	})
}

func TestGroupDataSourceLookup(t *testing.T) {
	b := installGroupBackend(t)
	description := "Existing group"
	b.groups["existing"] = api.Group{ID: "existing", Name: "Deployers & operators", Description: &description}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: groupTestProvider + `
data "rwx_group" "test" {
  name = "Deployers & operators"
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.rwx_group.test", "id", "existing"),
				resource.TestCheckResourceAttr("data.rwx_group.test", "description", description),
			),
		}},
	})
}

func TestGroupDataSourcePlanErrors(t *testing.T) {
	for _, tc := range []struct{ name, selector, message string }{
		{"missing ID", `id = "missing"`, "not found"},
		{"missing name", `name = "missing"`, "not found"},
		{"ambiguous", `name = "ambiguous"`, "multiple groups named"},
		{"empty ID", `id = ""`, "length must be at least 1"},
		{"neither", "", "Invalid Attribute Combination"},
		{"both", "id = \"one\"\nname = \"Deployers\"", "Invalid Attribute Combination"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			installGroupBackend(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps:                    []resource.TestStep{{Config: groupTestProvider + "\ndata \"rwx_group\" \"test\" {\n" + tc.selector + "\n}", PlanOnly: true, ExpectError: regexp.MustCompile(tc.message)}},
			})
		})
	}
}

func TestGroupResourceCreateDenied(t *testing.T) {
	b := installGroupBackend(t)
	b.setFailure(http.MethodPost, http.StatusForbidden)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    []resource.TestStep{{Config: groupTestConfig("Deployers", ""), ExpectError: regexp.MustCompile("group:manage")}},
	})
	if len(b.groups) != 0 {
		t.Fatal("denied create adopted a group")
	}
}

const groupTestProvider = `
provider "rwx" {
  host = "groups.test"
  access_token = "service-account-test-token"
}
`

func groupTestConfig(name, description string) string {
	return groupTestProvider + fmt.Sprintf(`
resource "rwx_group" "test" {
  name = %q
  %s
}
`, name, description)
}

type groupBackend struct {
	mu         sync.Mutex
	groups     map[string]api.Group
	nextID     int
	failMethod string
	failStatus int
}

func installGroupBackend(t *testing.T) *groupBackend {
	t.Helper()
	b := &groupBackend{groups: make(map[string]api.Group)}
	previous := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: b}
	t.Cleanup(func() { http.DefaultClient = previous })
	return b
}

func (b *groupBackend) setFailure(method string, status int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failMethod, b.failStatus = method, status
}

func (b *groupBackend) RoundTrip(req *http.Request) (*http.Response, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if req.Header.Get("Authorization") != "Bearer service-account-test-token" {
		return groupJSONResponse(http.StatusUnauthorized, map[string]any{"error": "Unauthorized"}), nil
	}
	if req.Method == b.failMethod {
		if b.failStatus == http.StatusConflict {
			return groupJSONResponse(b.failStatus, map[string]any{"errors": []string{"System groups cannot be edited or deleted."}}), nil
		}
		return groupJSONResponse(b.failStatus, map[string]any{"error_messages": []map[string]string{{"message": "Requires group:manage permission"}}}), nil
	}
	if req.URL.Path == "/mint/api/groups" {
		if req.Method == http.MethodGet {
			name := req.URL.Query().Get("name")
			if req.URL.Query().Get("cursor") == "" {
				groups := []api.Group{}
				if name == "ambiguous" {
					groups = append(groups, api.Group{ID: "first", Name: name})
				}
				return groupJSONResponse(http.StatusOK, map[string]any{"groups": groups, "pagination": map[string]any{"next_cursor": "next+/="}}), nil
			}
			if req.URL.Query().Get("cursor") != "next+/=" {
				return nil, fmt.Errorf("cursor not preserved")
			}
			groups := []api.Group{}
			for _, g := range b.groups {
				if g.Name == name {
					groups = append(groups, g)
				}
			}
			if name == "ambiguous" {
				groups = append(groups, api.Group{ID: "second", Name: name})
			}
			return groupJSONResponse(http.StatusOK, map[string]any{"groups": groups, "pagination": map[string]any{"next_cursor": nil}}), nil
		}
		if req.Method == http.MethodPost {
			b.nextID++
			return b.write(req, fmt.Sprintf("group-%d", b.nextID), http.StatusCreated)
		}
	}
	id := strings.TrimPrefix(req.URL.Path, "/mint/api/groups/")
	g, ok := b.groups[id]
	if !ok {
		return groupJSONResponse(http.StatusNotFound, nil), nil
	}
	switch req.Method {
	case http.MethodGet:
		return groupJSONResponse(http.StatusOK, map[string]any{"group": g}), nil
	case http.MethodPatch:
		return b.write(req, id, http.StatusOK)
	case http.MethodDelete:
		delete(b.groups, id)
		return groupJSONResponse(http.StatusNoContent, nil), nil
	}
	return nil, fmt.Errorf("unexpected request: %s %s", req.Method, req.URL)
}

func (b *groupBackend) write(req *http.Request, id string, status int) (*http.Response, error) {
	var body map[string]json.RawMessage
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return nil, err
	}
	if len(body) != 2 || body["name"] == nil || body["description"] == nil {
		return nil, fmt.Errorf("group writes must contain only name and description: %v", body)
	}
	var g api.Group
	if err := json.Unmarshal(body["name"], &g.Name); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body["description"], &g.Description); err != nil {
		return nil, err
	}
	g.ID = id
	b.groups[id] = g
	return groupJSONResponse(status, map[string]any{"group": g}), nil
}

func groupJSONResponse(status int, body any) *http.Response {
	encoded, _ := json.Marshal(body)
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(string(encoded)))}
}
