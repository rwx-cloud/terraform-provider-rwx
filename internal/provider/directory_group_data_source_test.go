package provider

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

type directoryGroupTransport func(*http.Request) (*http.Response, error)

func (f directoryGroupTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestDirectoryGroupDataSource(t *testing.T) {
	var mu sync.Mutex
	name := "Engineering"
	deleted := false
	previous := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: directoryGroupTransport(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		if req.Method != http.MethodGet || req.Header.Get("Authorization") != "Bearer service-account-test-token" {
			return nil, fmt.Errorf("unexpected request: %s %s", req.Method, req.URL)
		}
		status := http.StatusOK
		body := ""
		q := req.URL.Query()
		if q.Get("directory_id") != "directory-1" {
			status = http.StatusNotFound
		} else if req.URL.Path == "/mint/api/directory_groups/group-1" {
			if deleted {
				status = http.StatusNotFound
			} else {
				body = fmt.Sprintf(`{"directory_group":{"id":"group-1","directory_id":"directory-1","name":%q}}`, name)
			}
		} else if req.URL.Path == "/mint/api/directory_groups" {
			if q.Get("name") != name {
				body = `{"directory_groups":[],"pagination":{"next_cursor":null}}`
			} else if q.Get("cursor") == "" {
				body = `{"directory_groups":[],"pagination":{"next_cursor":"next"}}`
			} else {
				id := "group-1"
				if deleted {
					id = "replacement"
				}
				body = fmt.Sprintf(`{"directory_groups":[{"id":%q,"directory_id":"directory-1","name":%q}],"pagination":{"next_cursor":null}}`, id, name)
			}
		} else {
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	t.Cleanup(func() { http.DefaultClient = previous })
	byID := directoryGroupConfig(`directory_id = "directory-1"` + "\n" + `id = "group-1"`)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: directoryGroupConfig("directory_id = terraform_data.directory.output\nname = \"Engineering\"") + "\nresource \"terraform_data\" \"directory\" { input = \"directory-1\" }", Check: resource.TestCheckResourceAttr("data.rwx_directory_group.test", "id", "group-1")},
			{Config: directoryGroupConfig(`directory_id = "directory-1"` + "\n" + `name = "Engineering"`), Check: resource.TestCheckResourceAttr("data.rwx_directory_group.test", "id", "group-1")},
			{Config: byID, Check: resource.TestCheckResourceAttr("data.rwx_directory_group.test", "name", "Engineering")},
			{Config: byID, PlanOnly: true},
			{PreConfig: func() { mu.Lock(); name = "Renamed"; mu.Unlock() }, Config: byID, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.rwx_directory_group.test", "id", "group-1"), resource.TestCheckResourceAttr("data.rwx_directory_group.test", "name", "Renamed"))},
			{PreConfig: func() { mu.Lock(); deleted = true; mu.Unlock() }, Config: byID, PlanOnly: true, ExpectError: regexp.MustCompile("not found")},
		},
	})
}

func TestDirectoryGroupDataSourceInvalidPlan(t *testing.T) {
	for _, tt := range []struct{ name, attributes, error string }{
		{"missing directory", `name = "Engineering"`, "Missing required argument"},
		{"missing selector", `directory_id = "dir"`, "Invalid Attribute Combination"},
		{"both selectors", "directory_id = \"dir\"\nname = \"Engineering\"\nid = \"group\"", "Invalid Attribute Combination"},
		{"empty selector", "directory_id = \"dir\"\nname = \"\"", "Invalid Attribute Value Length"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps:                    []resource.TestStep{{Config: directoryGroupConfig(tt.attributes), PlanOnly: true, ExpectError: regexp.MustCompile(tt.error)}},
			})
		})
	}
}

func TestDirectoryGroupDataSourceLookupFailsDuringPlan(t *testing.T) {
	for _, tt := range []struct {
		name      string
		selector  string
		status    int
		body      string
		wantError string
	}{
		{"missing name", `name = "Missing"`, 200, `{"directory_groups":[]}`, "not found"},
		{"ambiguous name", `name = "Engineering"`, 200, `{"directory_groups":[{"id":"one","directory_id":"directory-1","name":"Engineering"},{"id":"two","directory_id":"directory-1","name":"Engineering"}]}`, "multiple directory groups"},
		{"cross directory ID", `id = "other-directory-group"`, 404, "", "not found"},
		{"cross organization ID", `id = "other-org-group"`, 404, "", "not found"},
		{"unauthorized", `id = "group-1"`, 401, `{"error":"Unauthorized"}`, "Unauthorized"},
		{"forbidden", `name = "Engineering"`, 403, `{"error":"org:access required"}`, "org:access required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			previous := http.DefaultClient
			http.DefaultClient = &http.Client{Transport: directoryGroupTransport(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tt.status, Status: http.StatusText(tt.status), Body: io.NopCloser(strings.NewReader(tt.body))}, nil
			})}
			t.Cleanup(func() { http.DefaultClient = previous })
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps:                    []resource.TestStep{{Config: directoryGroupConfig("directory_id = \"directory-1\"\n" + tt.selector), PlanOnly: true, ExpectError: regexp.MustCompile(tt.wantError)}},
			})
		})
	}
}

func directoryGroupConfig(attributes string) string {
	return `
provider "rwx" {
  host = "directory-groups.test"
  access_token = "service-account-test-token"
}
data "rwx_directory_group" "test" {
` + attributes + "\n}\n"
}
