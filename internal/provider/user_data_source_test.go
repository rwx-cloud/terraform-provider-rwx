package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

func TestUserDataSource(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer service-account-token" {
			t.Errorf("unexpected request: %s, authorization %q", r.Method, r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		selector := strings.TrimPrefix(r.URL.Path, "/mint/api/organization_users/")
		if selector == "lookup" {
			selector = r.URL.Query().Get("email")
		}
		switch selector {
		case "user-id", "Engineer+ci&ops@Example.com":
			_, _ = fmt.Fprint(w, `{"user":{"id":"user-id","identity_id":"identity-id","email":"engineer+ci&ops@example.com"}}`)
		case "denied-id", "denied@example.com":
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `{"error_messages":[{"message":"org:access required"}],"required_permission":"org:access"}`)
		case "unauthorized-id":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, `{"error":"Unauthorized"}`)
		case " ":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"errors":["email must be provided"]}`)
		default:
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
data "rwx_user" "test" {
  %s
}
output "user_id" { value = data.rwx_user.test.id }
output "identity_id" { value = data.rwx_user.test.identity_id }
output "email" { value = data.rwx_user.test.email }
`, strings.TrimPrefix(server.URL, "https://"), selector)
	}
	for _, test := range []struct{ name, selector, email string }{
		{"email", `email = "Engineer+ci&ops@Example.com"`, "Engineer+ci&ops@Example.com"},
		{"ID", `id = "user-id"`, "engineer+ci&ops@example.com"},
	} {
		t.Run(test.name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: config(test.selector),
						ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
							plancheck.ExpectKnownOutputValue("user_id", knownvalue.StringExact("user-id")),
							plancheck.ExpectKnownOutputValue("identity_id", knownvalue.StringExact("identity-id")),
							plancheck.ExpectKnownOutputValue("email", knownvalue.StringExact(test.email)),
						}},
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr("data.rwx_user.test", "id", "user-id"),
							resource.TestCheckResourceAttr("data.rwx_user.test", "identity_id", "identity-id"),
							resource.TestCheckResourceAttr("data.rwx_user.test", "email", test.email),
						),
					},
					{Config: config(test.selector), PlanOnly: true},
				},
			})
		})
	}
	for _, test := range []struct{ name, selector, error string }{
		{"missing email", `email = "missing@example.com"`, "not found"},
		{"foreign email", `email = "foreign@example.com"`, "not found"},
		{"foreign ID", `id = "foreign-id"`, "not found"},
		{"inactive user", `id = "inactive-id"`, "not found"},
		{"non-user identity ID", `id = "identity-id"`, "not found"},
		{"denied email", `email = "denied@example.com"`, "org:access required"},
		{"denied ID", `id = "denied-id"`, "org:access required"},
		{"unauthorized", `id = "unauthorized-id"`, "Unauthorized"},
		{"blank email", `email = " "`, "email must be provided"},
		{"no selector", ``, "Invalid Attribute Combination"},
		{"both selectors", "id = \"user-id\"\nemail = \"engineer@example.com\"", "Invalid Attribute Combination"},
		{"empty ID", `id = ""`, "Invalid Attribute Value Length"},
		{"empty email", `email = ""`, "Invalid Attribute Value Length"},
	} {
		t.Run(test.name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps:                    []resource.TestStep{{Config: config(test.selector), PlanOnly: true, ExpectError: regexp.MustCompile(test.error)}},
			})
		})
	}
	t.Run("unknown selector resolves at apply", func(t *testing.T) {
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{{
				Config: config(`id = terraform_data.selector.output`) + `
resource "terraform_data" "selector" { input = "user-id" }
`,
				Check: resource.TestCheckResourceAttr("data.rwx_user.test", "id", "user-id"),
			}},
		})
	})
}
