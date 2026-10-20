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

func TestGroupServiceAccountMembershipLifecycle(t *testing.T) {
	backend := newGroupServiceAccountMembershipBackend(t)
	backend.joins["group/account"] = true
	backend.joins["other-group/account"] = true
	backend.joins["group/other-account"] = true
	config := groupServiceAccountMembershipConfig("group", "account")
	check := func(group, account string) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("rwx_group_service_account_membership.test", "id", group+"/"+account),
			func(*terraform.State) error {
				backend.mu.Lock()
				defer backend.mu.Unlock()
				if len(backend.joins) != 3 || !backend.joins[group+"/"+account] || !backend.joins["other-group/account"] || !backend.joins["group/other-account"] {
					return fmt.Errorf("unexpected direct memberships: %#v", backend.joins)
				}
				return nil
			},
		)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config, Check: check("group", "account")},
			{Config: config, PlanOnly: true},
			{ResourceName: "rwx_group_service_account_membership.test", ImportState: true, ImportStateVerify: true},
			{Config: config, PlanOnly: true},
			{PreConfig: func() {
				backend.mu.Lock()
				defer backend.mu.Unlock()
				delete(backend.joins, "group/account")
			}, Config: config, PlanOnly: true, ExpectNonEmptyPlan: true},
			{Config: config, Check: check("group", "account")},
			{Config: groupServiceAccountMembershipConfig("replacement", "account"), Check: check("replacement", "account")},
			{Config: groupServiceAccountMembershipConfig("replacement", "replacement-account"), Check: check("replacement", "replacement-account")},
			{Config: groupServiceAccountMembershipConfig("replacement", "replacement-account"), PlanOnly: true},
		},
	})
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if len(backend.joins) != 2 || !backend.joins["other-group/account"] || !backend.joins["group/other-account"] {
		t.Fatalf("destroy changed unrelated memberships: %#v", backend.joins)
	}
}

func TestGroupServiceAccountMembershipFailures(t *testing.T) {
	backend := newGroupServiceAccountMembershipBackend(t)
	config := groupServiceAccountMembershipConfig("group", "account")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: groupServiceAccountMembershipConfig("", "account"), PlanOnly: true, ExpectError: regexp.MustCompile("Invalid Attribute Value Length")},
			{Config: groupServiceAccountMembershipConfig("group", ""), PlanOnly: true, ExpectError: regexp.MustCompile("Invalid Attribute Value Length")},
			{PreConfig: func() { backend.setFailure(http.MethodPost, 403) }, Config: config, ExpectError: regexp.MustCompile("Requires group:manage")},
			{PreConfig: func() { backend.setFailure(http.MethodPost, 422) }, Config: config, ExpectError: regexp.MustCompile("Unable to add service account")},
			{PreConfig: func() { backend.setFailure("", 0) }, Config: config},
			{PreConfig: func() { backend.setFailure(http.MethodGet, 401) }, Config: config, PlanOnly: true, ExpectError: regexp.MustCompile("Unauthorized")},
			{PreConfig: func() { backend.setFailure(http.MethodGet, 403) }, Config: config, PlanOnly: true, ExpectError: regexp.MustCompile("Requires org:access")},
			{PreConfig: func() { backend.setFailure(http.MethodGet, 500) }, Config: config, PlanOnly: true, ExpectError: regexp.MustCompile("Internal Server Error")},
			{PreConfig: func() { backend.setFailure(http.MethodDelete, 403) }, Config: config, Destroy: true, ExpectError: regexp.MustCompile("Requires group:manage")},
			{PreConfig: func() { backend.setFailure(http.MethodDelete, 409) }, Config: config, Destroy: true, ExpectError: regexp.MustCompile("no admin")},
			{PreConfig: func() { backend.setFailure("", 0) }, Config: config, PlanOnly: true},
			{ResourceName: "rwx_group_service_account_membership.test", ImportState: true, ImportStateId: "group/account/extra", ExpectError: regexp.MustCompile("Invalid group service account membership import ID")},
		},
	})
}

func TestGroupServiceAccountMembershipDeleteAbsent(t *testing.T) {
	ctx := context.Background()
	r := &GroupServiceAccountMembershipResource{client: api.Client{RoundTrip: func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}, nil
	}}}
	var schemaResponse frameworkresource.SchemaResponse
	r.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResponse)
	state := tfsdk.State{Schema: schemaResponse.Schema}
	diags := state.Set(ctx, &GroupServiceAccountMembershipResourceModel{
		ID: types.StringValue("group/account"), GroupID: types.StringValue("group"), ServiceAccountID: types.StringValue("account"),
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

func groupServiceAccountMembershipConfig(group, account string) string {
	return fmt.Sprintf(`
provider "rwx" {
  host = "group-service-account-memberships.test"
  access_token = "service-account-test-token"
}
resource "rwx_group_service_account_membership" "test" {
  group_id = %q
  service_account_id = %q
}
`, group, account)
}

type groupServiceAccountMembershipBackend struct {
	mu            sync.Mutex
	joins         map[string]bool
	failureMethod string
	failureStatus int
}

func newGroupServiceAccountMembershipBackend(t *testing.T) *groupServiceAccountMembershipBackend {
	t.Helper()
	b := &groupServiceAccountMembershipBackend{joins: make(map[string]bool)}
	previous := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: b}
	t.Cleanup(func() { http.DefaultClient = previous })
	return b
}

func (b *groupServiceAccountMembershipBackend) setFailure(method string, status int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failureMethod, b.failureStatus = method, status
}

func (b *groupServiceAccountMembershipBackend) RoundTrip(req *http.Request) (*http.Response, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	respond := func(status int, body any) (*http.Response, error) {
		encoded, err := json.Marshal(body)
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(string(encoded)))}, err
	}
	if req.Header.Get("Authorization") != "Bearer service-account-test-token" {
		return respond(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
	}
	if req.URL.Path != "/mint/api/group_service_account_memberships" {
		return nil, fmt.Errorf("unexpected request: %s %s", req.Method, req.URL)
	}
	if req.Method == b.failureMethod {
		message := http.StatusText(b.failureStatus)
		switch b.failureStatus {
		case 403:
			message = "Requires group:manage"
			if req.Method == http.MethodGet {
				message = "Requires org:access"
			}
		case 409:
			message = "That leaves the organization with no admin."
		case 422:
			message = "Unable to add service account"
		}
		return respond(b.failureStatus, map[string][]string{"errors": {message}})
	}
	var pair api.GroupServiceAccountMembership
	if req.Method == http.MethodGet {
		pair.GroupID, pair.ServiceAccountID = req.URL.Query().Get("group_id"), req.URL.Query().Get("service_account_id")
	} else if err := json.NewDecoder(req.Body).Decode(&pair); err != nil {
		return nil, err
	}
	if pair.GroupID == "" || pair.ServiceAccountID == "" {
		return respond(http.StatusBadRequest, nil)
	}
	id := pair.GroupID + "/" + pair.ServiceAccountID
	switch req.Method {
	case http.MethodPost:
		b.joins[id] = true
		return respond(http.StatusCreated, map[string]any{"group_service_account_membership": pair})
	case http.MethodGet:
		if !b.joins[id] {
			return respond(http.StatusNotFound, nil)
		}
		return respond(http.StatusOK, map[string]any{"group_service_account_membership": pair})
	case http.MethodDelete:
		if !b.joins[id] {
			return respond(http.StatusNotFound, nil)
		}
		delete(b.joins, id)
		return respond(http.StatusNoContent, nil)
	default:
		return nil, fmt.Errorf("unexpected method: %s", req.Method)
	}
}
