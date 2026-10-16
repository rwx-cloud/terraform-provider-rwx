package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"
)

func TestGroupRelationshipResourceLifecycle(t *testing.T) {
	backend := newGroupRelationshipBackend(t)
	backend.edges["other-parent/child"] = true
	backend.edges["parent/other-child"] = true
	config := groupRelationshipConfig("parent", "child")
	check := func(parent, child string) resource.TestCheckFunc {
		return func(state *terraform.State) error {
			backend.mu.Lock()
			defer backend.mu.Unlock()
			id := parent + "/" + child
			if !backend.edges[id] || len(backend.edges) != 3 || !backend.edges["other-parent/child"] || !backend.edges["parent/other-child"] {
				return fmt.Errorf("unexpected relationships: %#v", backend.edges)
			}
			attrs := state.RootModule().Resources["rwx_group_relationship.test"].Primary.Attributes
			if attrs["id"] != id || attrs["parent_group_id"] != parent || attrs["child_group_id"] != child {
				return fmt.Errorf("unexpected state: %#v", attrs)
			}
			return nil
		}
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config, Check: check("parent", "child")},
			{Config: config, PlanOnly: true},
			{ResourceName: "rwx_group_relationship.test", ImportState: true, ImportStateVerify: true},
			{Config: config, PlanOnly: true},
			{
				PreConfig: func() {
					backend.mu.Lock()
					defer backend.mu.Unlock()
					delete(backend.edges, "parent/child")
				},
				Config: config, PlanOnly: true, ExpectNonEmptyPlan: true,
			},
			{Config: config, Check: check("parent", "child")},
			{Config: groupRelationshipConfig("replacement-parent", "child"), Check: check("replacement-parent", "child")},
			{Config: groupRelationshipConfig("replacement-parent", "replacement-child"), Check: check("replacement-parent", "replacement-child")},
			{Config: groupRelationshipConfig("replacement-parent", "replacement-child"), PlanOnly: true},
		},
	})
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if len(backend.edges) != 2 || !backend.edges["other-parent/child"] || !backend.edges["parent/other-child"] {
		t.Fatalf("destroy changed unrelated relationships: %#v", backend.edges)
	}
}

func TestGroupRelationshipResourceFailures(t *testing.T) {
	backend := newGroupRelationshipBackend(t)
	config := groupRelationshipConfig("parent", "child")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: groupRelationshipConfig("child", "parent"), ExpectError: regexp.MustCompile("already above the parent")},
			{Config: config},
			{
				PreConfig: func() { backend.setFailure(http.MethodGet, http.StatusUnauthorized) },
				Config:    config, PlanOnly: true, ExpectError: regexp.MustCompile("Unauthorized"),
			},
			{
				PreConfig: func() { backend.setFailure(http.MethodGet, http.StatusForbidden) },
				Config:    config, PlanOnly: true, ExpectError: regexp.MustCompile("requires org:access"),
			},
			{
				PreConfig: func() { backend.setFailure(http.MethodDelete, http.StatusConflict) },
				Config:    config, Destroy: true, ExpectError: regexp.MustCompile("leaves the organization with no admin"),
			},
			{
				PreConfig: func() { backend.setFailure("", 0) },
				Config:    config, PlanOnly: true,
				Check: resource.TestCheckResourceAttr("rwx_group_relationship.test", "id", "parent/child"),
			},
			{ResourceName: "rwx_group_relationship.test", ImportState: true, ImportStateId: "parent/child/extra", ExpectError: regexp.MustCompile("Invalid group relationship import ID")},
		},
	})
}

func TestGroupRelationshipResourceDeleteAlreadyAbsent(t *testing.T) {
	ctx := context.Background()
	r := &GroupRelationshipResource{client: api.Client{RoundTrip: func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}, nil
	}}}
	var schemaResponse frameworkresource.SchemaResponse
	r.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResponse)
	state := tfsdk.State{Schema: schemaResponse.Schema}
	diags := state.Set(ctx, &GroupRelationshipResourceModel{
		ID:            types.StringValue("parent/child"),
		ParentGroupID: types.StringValue("parent"),
		ChildGroupID:  types.StringValue("child"),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	var response frameworkresource.DeleteResponse
	r.Delete(ctx, frameworkresource.DeleteRequest{State: state}, &response)
	if response.Diagnostics.HasError() {
		t.Fatal(response.Diagnostics)
	}
}

func groupRelationshipConfig(parent, child string) string {
	return fmt.Sprintf(`
provider "rwx" {
  host = "group-relationships.test"
  access_token = "service-account-test-token"
}
resource "rwx_group_relationship" "test" {
  parent_group_id = %q
  child_group_id = %q
}
`, parent, child)
}

type groupRelationshipBackend struct {
	mu            sync.Mutex
	edges         map[string]bool
	failureMethod string
	failureStatus int
}

func newGroupRelationshipBackend(t *testing.T) *groupRelationshipBackend {
	t.Helper()
	backend := &groupRelationshipBackend{edges: make(map[string]bool)}
	previous := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: backend}
	t.Cleanup(func() { http.DefaultClient = previous })
	return backend
}

func (b *groupRelationshipBackend) setFailure(method string, status int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failureMethod, b.failureStatus = method, status
}

func (b *groupRelationshipBackend) RoundTrip(req *http.Request) (*http.Response, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	respond := func(status int, body any) (*http.Response, error) {
		encoded, err := json.Marshal(body)
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(string(encoded)))}, err
	}
	if req.Header.Get("Authorization") != "Bearer service-account-test-token" {
		return respond(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
	}
	if req.URL.Path != "/mint/api/group_relationships" {
		return nil, fmt.Errorf("unexpected request: %s %s", req.Method, req.URL)
	}
	if req.Method == b.failureMethod {
		message := "That leaves the organization with no admin."
		switch b.failureStatus {
		case http.StatusUnauthorized:
			message = "Unauthorized"
		case http.StatusForbidden:
			message = "It requires org:access"
		}
		return respond(b.failureStatus, map[string][]string{"errors": {message}})
	}
	var pair struct {
		Parent string `json:"parent_group_id"`
		Child  string `json:"child_group_id"`
	}
	if req.Method == http.MethodGet {
		pair.Parent, pair.Child = req.URL.Query().Get("parent_group_id"), req.URL.Query().Get("child_group_id")
	} else if err := json.NewDecoder(req.Body).Decode(&pair); err != nil {
		return nil, err
	}
	if pair.Parent == "" || pair.Child == "" {
		return respond(http.StatusBadRequest, nil)
	}
	id := pair.Parent + "/" + pair.Child
	switch req.Method {
	case http.MethodPost:
		if id == "child/parent" {
			return respond(http.StatusUnprocessableEntity, map[string][]string{"errors": {"Child is already above the parent in the group hierarchy"}})
		}
		b.edges[id] = true
		return respond(http.StatusCreated, map[string]any{"group_relationship": pair})
	case http.MethodGet:
		if !b.edges[id] {
			return respond(http.StatusNotFound, nil)
		}
		return respond(http.StatusOK, map[string]any{"group_relationship": pair})
	case http.MethodDelete:
		if !b.edges[id] {
			return respond(http.StatusNotFound, nil)
		}
		delete(b.edges, id)
		return respond(http.StatusNoContent, nil)
	default:
		return nil, fmt.Errorf("unexpected method: %s", req.Method)
	}
}
