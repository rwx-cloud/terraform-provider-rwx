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

func TestGroupUserMembershipResourceLifecycle(t *testing.T) {
	backend := newGroupUserMembershipBackend(t)
	backend.manual["group/other-user"] = true
	backend.manual["other-group/user"] = true
	config := groupUserMembershipConfig("group", "user")
	check := func(group, user string) resource.TestCheckFunc {
		return func(state *terraform.State) error {
			backend.mu.Lock()
			defer backend.mu.Unlock()
			id := group + "/" + user
			if len(backend.manual) != 3 || !backend.manual[id] || !backend.manual["group/other-user"] || !backend.manual["other-group/user"] {
				return fmt.Errorf("unexpected manual memberships: %#v", backend.manual)
			}
			attrs := state.RootModule().Resources["rwx_group_user_membership.test"].Primary.Attributes
			if attrs["id"] != id || attrs["group_id"] != group || attrs["user_id"] != user {
				return fmt.Errorf("unexpected state: %#v", attrs)
			}
			return nil
		}
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config, Check: check("group", "user")},
			{Config: config, PlanOnly: true},
			{ResourceName: "rwx_group_user_membership.test", ImportState: true, ImportStateVerify: true},
			{Config: config, PlanOnly: true},
			{PreConfig: func() {
				backend.mu.Lock()
				defer backend.mu.Unlock()
				delete(backend.manual, "group/user")
			}, Config: config, PlanOnly: true, ExpectNonEmptyPlan: true},
			{Config: config, Check: check("group", "user")},
			{Config: groupUserMembershipConfig("replacement-group", "user"), Check: check("replacement-group", "user")},
			{Config: groupUserMembershipConfig("replacement-group", "replacement-user"), Check: check("replacement-group", "replacement-user")},
			{Config: groupUserMembershipConfig("replacement-group", "replacement-user"), PlanOnly: true},
		},
	})
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if len(backend.manual) != 2 || !backend.manual["group/other-user"] || !backend.manual["other-group/user"] {
		t.Fatalf("destroy changed unrelated manual memberships: %#v", backend.manual)
	}
	if len(backend.directory) != 1 || !backend.directory["group/user"] || len(backend.inherited) != 1 || !backend.inherited["group/user"] {
		t.Fatalf("changed non-manual memberships: directory=%#v inherited=%#v", backend.directory, backend.inherited)
	}
}

func TestGroupUserMembershipResourceFailures(t *testing.T) {
	backend := newGroupUserMembershipBackend(t)
	config := groupUserMembershipConfig("group", "user")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: groupUserMembershipConfig("foreign-group", "user"), ExpectError: regexp.MustCompile("not found")},
			{PreConfig: func() { backend.setFailure(http.MethodPost, 422) }, Config: config, ExpectError: regexp.MustCompile("Directory sync manages user membership")},
			{PreConfig: func() { backend.setFailure("", 0) }, Config: config},
			{PreConfig: func() { backend.setFailure(http.MethodGet, 401) }, Config: config, PlanOnly: true, ExpectError: regexp.MustCompile("Unauthorized")},
			{PreConfig: func() { backend.setFailure(http.MethodGet, 403) }, Config: config, PlanOnly: true, ExpectError: regexp.MustCompile("requires org:access")},
			{PreConfig: func() { backend.setFailure(http.MethodDelete, 403) }, Config: config, Destroy: true, ExpectError: regexp.MustCompile("requires group:manage")},
			{PreConfig: func() { backend.setFailure(http.MethodDelete, 422) }, Config: config, Destroy: true, ExpectError: regexp.MustCompile("Directory sync manages user membership")},
			{PreConfig: func() { backend.setFailure(http.MethodDelete, 409) }, Config: config, Destroy: true, ExpectError: regexp.MustCompile("leaves the organization with no admin")},
			{PreConfig: func() { backend.setFailure("", 0) }, Config: config, PlanOnly: true, Check: resource.TestCheckResourceAttr("rwx_group_user_membership.test", "id", "group/user")},
			{ResourceName: "rwx_group_user_membership.test", ImportState: true, ImportStateId: "group/user/extra", ExpectError: regexp.MustCompile("Invalid group user membership import ID")},
			{ResourceName: "rwx_group_user_membership.test", ImportState: true, ImportStateId: "/user", ExpectError: regexp.MustCompile("Invalid group user membership import ID")},
		},
	})
}

func TestGroupUserMembershipResourceImportNonManual(t *testing.T) {
	newGroupUserMembershipBackend(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: groupUserMembershipConfig("group", "user"), ResourceName: "rwx_group_user_membership.test", ImportState: true, ImportStateId: "group/user", ExpectError: regexp.MustCompile("Cannot import non-existent remote object")},
		},
	})
}

func TestGroupUserMembershipResourceDeleteAlreadyAbsent(t *testing.T) {
	ctx := context.Background()
	r := &GroupUserMembershipResource{client: api.Client{RoundTrip: func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}, nil
	}}}
	var schemaResponse frameworkresource.SchemaResponse
	r.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResponse)
	state := tfsdk.State{Schema: schemaResponse.Schema}
	if diags := state.Set(ctx, &GroupUserMembershipResourceModel{
		ID: types.StringValue("group/user"), GroupID: types.StringValue("group"), UserID: types.StringValue("user"),
	}); diags.HasError() {
		t.Fatal(diags)
	}
	var response frameworkresource.DeleteResponse
	r.Delete(ctx, frameworkresource.DeleteRequest{State: state}, &response)
	if response.Diagnostics.HasError() {
		t.Fatal(response.Diagnostics)
	}
}

func groupUserMembershipConfig(group, user string) string {
	return fmt.Sprintf(`
provider "rwx" {
  host = "group-user-memberships.test"
  access_token = "service-account-test-token"
}
resource "rwx_group_user_membership" "test" {
  group_id = %q
  user_id = %q
}
`, group, user)
}

type groupUserMembershipBackend struct {
	mu            sync.Mutex
	manual        map[string]bool
	directory     map[string]bool
	inherited     map[string]bool
	failureMethod string
	failureStatus int
}

func newGroupUserMembershipBackend(t *testing.T) *groupUserMembershipBackend {
	t.Helper()
	backend := &groupUserMembershipBackend{
		manual: make(map[string]bool), directory: map[string]bool{"group/user": true}, inherited: map[string]bool{"group/user": true},
	}
	previous := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: backend}
	t.Cleanup(func() { http.DefaultClient = previous })
	return backend
}

func (b *groupUserMembershipBackend) setFailure(method string, status int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failureMethod, b.failureStatus = method, status
}

func (b *groupUserMembershipBackend) RoundTrip(req *http.Request) (*http.Response, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	respond := func(status int, body any) (*http.Response, error) {
		encoded, err := json.Marshal(body)
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(string(encoded)))}, err
	}
	if req.Header.Get("Authorization") != "Bearer service-account-test-token" {
		return respond(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
	}
	if req.URL.Path != "/mint/api/group_user_memberships" {
		return nil, fmt.Errorf("unexpected request: %s %s", req.Method, req.URL)
	}
	if req.Method == b.failureMethod {
		message := "That leaves the organization with no admin."
		switch b.failureStatus {
		case http.StatusUnauthorized:
			message = "Unauthorized"
		case http.StatusForbidden:
			message = "It requires group:manage"
			if req.Method == http.MethodGet {
				message = "It requires org:access"
			}
		case http.StatusUnprocessableEntity:
			message = "Directory sync manages user membership in every RWX group."
		}
		return respond(b.failureStatus, map[string][]string{"errors": {message}})
	}
	var pair api.GroupUserMembership
	if req.Method == http.MethodGet {
		pair.GroupID, pair.UserID = req.URL.Query().Get("group_id"), req.URL.Query().Get("user_id")
	} else if err := json.NewDecoder(req.Body).Decode(&pair); err != nil {
		return nil, err
	}
	if pair.GroupID == "" || pair.UserID == "" {
		return respond(http.StatusBadRequest, nil)
	}
	if pair.GroupID == "foreign-group" {
		return respond(http.StatusNotFound, nil)
	}
	id := pair.GroupID + "/" + pair.UserID
	switch req.Method {
	case http.MethodPost:
		b.manual[id] = true
		return respond(http.StatusCreated, map[string]any{"group_user_membership": pair})
	case http.MethodGet:
		if !b.manual[id] {
			return respond(http.StatusNotFound, nil)
		}
		return respond(http.StatusOK, map[string]any{"group_user_membership": pair})
	case http.MethodDelete:
		if !b.manual[id] {
			return respond(http.StatusNotFound, nil)
		}
		delete(b.manual, id)
		return respond(http.StatusNoContent, nil)
	default:
		return nil, fmt.Errorf("unexpected method: %s", req.Method)
	}
}
