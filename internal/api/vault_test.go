package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestCreateVault(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("method = %q, want POST", req.Method)
		}
		if req.URL.String() != "/mint/api/vaults" {
			t.Fatalf("URL = %q, want /mint/api/vaults", req.URL.String())
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(body), `{"name":"deploys","unlocked":true,"approvals_enabled":true,"required_approvals":2,"repository_permissions":[{"repository_slug":"rwx-cloud/cloud","branch_pattern":"main"}]}`; got != want {
			t.Fatalf("body = %s, want %s", got, want)
		}

		return jsonResponse(http.StatusCreated, `{"vault":{"id":"vault-id","name":"deploys","lock_status":"unlocked","approvals_enabled":true,"required_approvals":2,"repository_permissions":[{"repository_slug":"rwx-cloud/cloud","branch_pattern":"main"}],"oidc_subject":"org:example:vault:deploys"}}`), nil
	}}

	vault, err := client.CreateVault(Vault{
		Name:              "deploys",
		LockStatus:        "unlocked",
		ApprovalsEnabled:  true,
		RequiredApprovals: 2,
		RepositoryPermissions: []VaultRepositoryPermission{
			{RepositorySlug: "rwx-cloud/cloud", BranchPattern: "main"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if vault.ID != "vault-id" || vault.OIDCSubject != "org:example:vault:deploys" || !vault.ApprovalsEnabled || vault.RequiredApprovals != 2 {
		t.Fatalf("unexpected vault: %#v", vault)
	}
}

func TestGetVaultNotFound(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if got, want := req.URL.String(), "/mint/api/vaults/vault-id"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		return jsonResponse(http.StatusNotFound, `{}`), nil
	}}

	_, err := client.GetVault("vault-id")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestUpdateVaultApprovalSettings(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(body), `{"name":"deploys","unlocked":false,"approvals_enabled":true,"required_approvals":3,"repository_permissions":null}`; got != want {
			t.Fatalf("body = %s, want %s", got, want)
		}

		return jsonResponse(http.StatusOK, `{"vault":{"id":"vault-id","name":"deploys","lock_status":"locked","approvals_enabled":true,"required_approvals":3,"repository_permissions":[]}}`), nil
	}}

	vault, err := client.UpdateVault(Vault{
		ID:                "vault-id",
		Name:              "deploys",
		ApprovalsEnabled:  true,
		RequiredApprovals: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !vault.ApprovalsEnabled || vault.RequiredApprovals != 3 {
		t.Fatalf("unexpected vault: %#v", vault)
	}
}

func TestUpdateVaultConflict(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPatch {
			t.Fatalf("method = %q, want PATCH", req.Method)
		}
		if got, want := req.URL.String(), "/mint/api/vaults/vault-id"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		return jsonResponse(http.StatusConflict, `{"errors":["Name has already been taken"]}`), nil
	}}

	_, err := client.UpdateVault(Vault{ID: "vault-id", Name: "existing", LockStatus: "locked"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "Name has already been taken") {
		t.Fatalf("error = %q, want API error message", err)
	}
}

func TestDeleteVaultProtected(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodDelete {
			t.Fatalf("method = %q, want DELETE", req.Method)
		}
		return jsonResponse(http.StatusConflict, `{"errors":["The default vault cannot be deleted"]}`), nil
	}}

	err := client.DeleteVault("vault-id")
	if err == nil || !strings.Contains(err.Error(), "default vault") {
		t.Fatalf("error = %v, want protected vault message", err)
	}
}

func TestFindVaultByName(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "/mint/api/vaults?limit=100":
			return jsonResponse(http.StatusOK, `{"vaults":[{"id":"first-id","name":"first"}],"pagination":{"next_cursor":"page2"}}`), nil
		case "/mint/api/vaults?cursor=page2&limit=100":
			return jsonResponse(http.StatusOK, `{"vaults":[{"id":"target-id","name":"target"}],"pagination":{"next_cursor":null}}`), nil
		default:
			t.Fatalf("unexpected URL %q", req.URL.String())
			return nil, nil
		}
	}}

	vault, err := client.FindVaultByName("target")
	if err != nil {
		t.Fatal(err)
	}
	if vault.ID != "target-id" {
		t.Fatalf("vault ID = %q, want target-id", vault.ID)
	}
}

func TestListPagination(t *testing.T) {
	for _, collection := range []struct {
		key      string
		endpoint string
		list     func(Client) (any, error)
		want     any
	}{
		{"vaults", "/mint/api/vaults", func(c Client) (any, error) { return c.ListVaults() }, []Vault{{ID: "first"}, {ID: "last"}}},
		{"access_grants", "/mint/api/vaults/vault-id/access_grants", func(c Client) (any, error) { return c.ListAccessGrants("vault-id") }, []AccessGrant{{ID: "first"}, {ID: "last"}}},
	} {
		t.Run(collection.key, func(t *testing.T) {
			for _, scenario := range []struct {
				name    string
				pages   []string
				status  int
				failure error
				wantErr string
			}{
				{name: "multiple pages", pages: []string{`[{"id":"first"}]`, `[]`, `[{"id":"last"}]`}},
				{name: "empty final page", pages: []string{`[{"id":"first"},{"id":"last"}]`, `[]`}},
				{name: "empty collection", pages: []string{`[]`}},
				{name: "later HTTP error", pages: []string{`[{"id":"first"}]`, `[]`}, status: http.StatusForbidden, wantErr: "denied"},
				{name: "later invalid JSON", pages: []string{`[{"id":"first"}]`, `broken`}, wantErr: "unable to decode JSON response"},
				{name: "later transport error", pages: []string{`[{"id":"first"}]`, `[]`}, failure: errors.New("connection lost"), wantErr: "connection lost"},
			} {
				t.Run(scenario.name, func(t *testing.T) {
					page := 0
					client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
						if req.Method != http.MethodGet || req.URL.Path != collection.endpoint || req.URL.Query().Get("limit") != "100" {
							t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
						}
						cursor := fmt.Sprintf("page+/%d==&", page)
						if page == 0 {
							cursor = ""
						}
						if page >= len(scenario.pages) || req.URL.Query().Get("cursor") != cursor {
							t.Fatalf("unexpected page request: %s", req.URL)
						}
						body := scenario.pages[page]
						page++
						if page > 1 && scenario.failure != nil {
							return nil, scenario.failure
						}
						if page > 1 && scenario.status != 0 {
							return jsonResponse(scenario.status, `{"errors":["denied"]}`), nil
						}
						next := "null"
						if page < len(scenario.pages) {
							next = fmt.Sprintf("%q", fmt.Sprintf("page+/%d==&", page))
						}
						return jsonResponse(http.StatusOK, fmt.Sprintf(`{"%s":%s,"pagination":{"next_cursor":%s,"limit":100}}`, collection.key, body, next)), nil
					}}

					got, err := collection.list(client)
					if scenario.wantErr != "" {
						if err == nil || !strings.Contains(err.Error(), scenario.wantErr) || !reflect.ValueOf(got).IsNil() {
							t.Fatalf("got %v, %v; want nil results and error containing %q", got, err, scenario.wantErr)
						}
						return
					}
					if err != nil || page != len(scenario.pages) {
						t.Fatalf("got %v after %d pages", err, page)
					}
					if scenario.name == "empty collection" {
						if reflect.ValueOf(got).Len() != 0 {
							t.Fatalf("got %v, want empty collection", got)
						}
					} else if !reflect.DeepEqual(got, collection.want) {
						t.Fatalf("got %#v, want %#v", got, collection.want)
					}
				})
			}
		})
	}
}
