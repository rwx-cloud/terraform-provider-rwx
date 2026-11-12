package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestManagementTransport(t *testing.T) {
	for _, tc := range []struct {
		name, agent string
		pulumi      bool
	}{
		{"terraform", "terraform-provider-rwx/test", false},
		{"pulumi", "pulumi-rwx/test", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			intent := ""
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				expectedIntent := intent
				if req.Method == http.MethodGet || (strings.HasSuffix(req.URL.Path, "/terraform_management") && tc.pulumi) {
					expectedIntent = "claim"
				}
				if req.Header.Get("Authorization") != "Bearer fixture-token" || req.UserAgent() != tc.agent {
					t.Errorf("incorrect auth or caller attribution: %v", req.Header)
				}
				for header := range req.Header {
					if strings.HasPrefix(header, "X-Rwx-") && header != http.CanonicalHeaderKey(TerraformManagementHeader) && header != http.CanonicalHeaderKey("X-RWX-Pulumi-Management") {
						t.Errorf("unexpected RWX header: %s", header)
					}
				}
				terraform, pulumi := expectedIntent, ""
				if tc.pulumi {
					terraform, pulumi = "", expectedIntent
				}
				if req.Header.Get(TerraformManagementHeader) != terraform || req.Header.Get("X-RWX-Pulumi-Management") != pulumi {
					t.Errorf("intent headers: %v", req.Header)
				}
				switch {
				case strings.HasSuffix(req.URL.Path, "/terraform_management"):
					w.WriteHeader(204)
				case req.Method == http.MethodDelete:
					w.WriteHeader(204)
				case req.Method == http.MethodPost:
					w.WriteHeader(201)
					_, _ = fmt.Fprint(w, `{"group":{"id":"group-one","name":"one"}}`)
				default:
					_, _ = fmt.Fprint(w, `{"group":{"id":"group-one","name":"one"},"permissions":[]}`)
				}
			}))
			defer server.Close()
			previous := http.DefaultClient
			http.DefaultClient = server.Client()
			defer func() { http.DefaultClient = previous }()
			client, err := NewClient(Config{Host: strings.TrimPrefix(server.URL, "https://"), AccessToken: "fixture-token", Version: "test", Pulumi: tc.pulumi})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.GetGroup("group-one"); err != nil {
				t.Fatal(err)
			}
			if err := client.ClaimTerraformManagement(context.Background(), "/mint/api/groups/group-one"); err != nil {
				t.Fatal(err)
			}
			intent = "claim"
			if _, err := client.CreateGroup(Group{Name: "one"}); err != nil {
				t.Fatal(err)
			}
			intent = "release"
			if err := client.ClearGroupPermissions("group-one"); err != nil {
				t.Fatal(err)
			}
			intent = ""
			if err := client.DeleteGroup("group-one"); err != nil {
				t.Fatal(err)
			}
			req, err := http.NewRequest(http.MethodPatch, "/mint/api/groups/group-one", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := client.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			if err := resp.Body.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
