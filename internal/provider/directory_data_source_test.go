package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestDirectoryDataSource(t *testing.T) {
	var name atomic.Value
	var deleted atomic.Bool
	name.Store("Engineering & IT")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer service-account-token" {
			t.Errorf("unexpected request: %s, authorization %q", r.Method, r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/mint/api/directories/local-id":
			if deleted.Load() {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = fmt.Fprintf(w, `{"directory":{"id":"local-id","name":%q}}`, name.Load())
		case "/mint/api/directories/foreign-id":
			w.WriteHeader(http.StatusNotFound)
		case "/mint/api/directories/denied-id":
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `{"error":"org:access required"}`)
		case "/mint/api/directories":
			switch r.URL.Query().Get("name") {
			case "Engineering & IT":
				if r.URL.Query().Get("cursor") == "" {
					_, _ = fmt.Fprint(w, `{"directories":[],"pagination":{"next_cursor":"opaque+/="}}`)
				} else if r.URL.Query().Get("cursor") == "opaque+/=" {
					_, _ = fmt.Fprint(w, `{"directories":[{"id":"local-id","name":"Engineering & IT"}],"pagination":{"next_cursor":null}}`)
				} else {
					t.Errorf("unexpected cursor %q", r.URL.Query().Get("cursor"))
				}
			case "duplicate":
				if r.URL.Query().Get("cursor") == "" {
					_, _ = fmt.Fprint(w, `{"directories":[{"id":"one","name":"duplicate"}],"pagination":{"next_cursor":"second"}}`)
				} else {
					_, _ = fmt.Fprint(w, `{"directories":[{"id":"two","name":"duplicate"}],"pagination":{"next_cursor":null}}`)
				}
			case "denied":
				w.WriteHeader(http.StatusForbidden)
				_, _ = fmt.Fprint(w, `{"error":"org:access required"}`)
			default:
				_, _ = fmt.Fprint(w, `{"directories":[],"pagination":{"next_cursor":null}}`)
			}
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	previousClient := http.DefaultClient
	http.DefaultClient = server.Client()
	t.Cleanup(func() { http.DefaultClient = previousClient })
	config := func(selector string) string {
		return fmt.Sprintf(`
provider "rwx" {
  host = %q
  access_token = "service-account-token"
}
data "rwx_directory" "test" {
  %s
}
`, strings.TrimPrefix(server.URL, "https://"), selector)
	}

	t.Run("lookup and rename", func(t *testing.T) {
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{
					Config: config(`name = "Engineering & IT"`),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("data.rwx_directory.test", "id", "local-id"),
						resource.TestCheckResourceAttr("data.rwx_directory.test", "name", "Engineering & IT"),
					),
				},
				{Config: config(`name = "Engineering & IT"`), PlanOnly: true},
				{
					Config: config(`id = "local-id"`),
					Check:  resource.TestCheckResourceAttr("data.rwx_directory.test", "name", "Engineering & IT"),
				},
				{
					PreConfig: func() { name.Store("Renamed") },
					Config:    config(`id = "local-id"`),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("data.rwx_directory.test", "id", "local-id"),
						resource.TestCheckResourceAttr("data.rwx_directory.test", "name", "Renamed"),
					),
				},
				{Config: config(`id = "local-id"`), PlanOnly: true},
				{
					Config: config(`id = terraform_data.selector.output`) + `
resource "terraform_data" "selector" {
  input = "local-id"
}
`,
					Check: resource.TestCheckResourceAttr("data.rwx_directory.test", "id", "local-id"),
				},
				{
					PreConfig:   func() { deleted.Store(true) },
					Config:      config(`id = "local-id"`),
					PlanOnly:    true,
					ExpectError: regexp.MustCompile("not found"),
				},
			},
		})
	})
	for _, test := range []struct{ name, selector, error string }{
		{"missing name", `name = "missing"`, "not found"},
		{"foreign ID", `id = "foreign-id"`, "not found"},
		{"ambiguous across pages", `name = "duplicate"`, "multiple directories named"},
		{"denied name", `name = "denied"`, "org:access required"},
		{"denied ID", `id = "denied-id"`, "org:access required"},
		{"no selector", ``, "Invalid Attribute Combination"},
		{"both selectors", "id = \"local-id\"\nname = \"Renamed\"", "Invalid Attribute Combination"},
		{"empty ID", `id = ""`, "Invalid Attribute Value Length"},
		{"empty name", `name = ""`, "Invalid Attribute Value Length"},
	} {
		t.Run(test.name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps:                    []resource.TestStep{{Config: config(test.selector), PlanOnly: true, ExpectError: regexp.MustCompile(test.error)}},
			})
		})
	}
}
