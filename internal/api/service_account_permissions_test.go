package api

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestServiceAccountPermissionsClient(t *testing.T) {
	resourceID := "vault-one"
	want := []Permission{{Key: "run:read"}, {Key: "vault:manage", ResourceID: &resourceID}}
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if req.URL.EscapedPath() != "/mint/api/service_accounts/account%2Fone/permissions" {
			t.Fatalf("unexpected path %s", req.URL.EscapedPath())
		}
		if req.Method == http.MethodPut {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != `{"permissions":[]}` {
				t.Fatalf("clear body = %s", body)
			}
			if req.Header.Get("Content-Type") != "application/json" {
				t.Fatal("missing JSON content type")
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"permissions":[{"key":"run:read"},{"key":"vault:manage","resource_id":"vault-one"}]}`))}, nil
	}}
	got, err := client.GetServiceAccountPermissions("account/one")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, %v", got, err)
	}
	if _, err := client.ReplaceServiceAccountPermissions("account/one", nil); err != nil {
		t.Fatal(err)
	}
}

func TestPermissionAPIErrors(t *testing.T) {
	for _, test := range []struct {
		status   int
		body     string
		notFound bool
		message  string
	}{
		{404, ``, true, "not found"},
		{401, `{"error":"Unauthorized"}`, false, "Unauthorized"},
		{403, `{"error_messages":[{"message":"manage permission required"}]}`, false, "manage permission required"},
		{422, `{"errors":["replacement refused"]}`, false, "replacement refused"},
		{200, `{`, false, "unable to decode JSON response"},
	} {
		t.Run(test.message, func(t *testing.T) {
			client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body))}, nil
			}}
			for _, call := range []func() error{
				func() error { _, err := client.GetServiceAccountPermissions("account"); return err },
				func() error { _, err := client.ReplaceServiceAccountPermissions("account", []Permission{}); return err },
				func() error {
					_, err := client.GetPermissionCatalog(url.Values{"service_account_id": {"account"}})
					return err
				},
			} {
				err := call()
				if err == nil || errors.Is(err, ErrNotFound) != test.notFound || !strings.Contains(err.Error(), test.message) {
					t.Fatalf("unexpected error %v", err)
				}
			}
		})
	}
}

func TestPermissionCatalogClient(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Path != "/mint/api/permission_catalog" || req.URL.Query().Get("service_account_id") != "account&one" {
			t.Fatalf("unexpected request %s %s", req.Method, req.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"permissions":[{"key":"vault:manage","supports_resource_scope":true,"scope_resource":"vault","grantable":{"unscoped":false,"resource_ids":["vault-one"]}}],"unremovable_permissions":["vault:manage(id: vault-two)"]}`))}, nil
	}}
	catalog, err := client.GetPermissionCatalog(url.Values{"service_account_id": {"account&one"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Permissions) != 1 || !catalog.Permissions[0].SupportsResourceScope || catalog.Permissions[0].Grantable.Unscoped || !reflect.DeepEqual(catalog.Permissions[0].Grantable.ResourceIDs, []string{"vault-one"}) || !reflect.DeepEqual(catalog.UnremovablePermissions, []string{"vault:manage(id: vault-two)"}) {
		t.Fatalf("unexpected catalog %#v", catalog)
	}
}
