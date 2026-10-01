package api

import (
	"errors"
	"io"
	"net/http"
	"testing"
)

func TestListAccessGrants(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if got, want := req.URL.String(), "/mint/api/vaults/vault-id/access_grants"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		return jsonResponse(http.StatusOK, `{"access_grants":[{"id":"user-grant","vault":{"id":"vault-id","name":"production"},"principal":{"type":"user","id":"user-id","email":"user@example.com"},"expires_at":"2026-12-01T15:00:00.123456Z"},{"id":"service-account-grant","vault":{"id":"vault-id","name":"production"},"principal":{"type":"service_account","id":"service-account-id","name":"release-bot"},"expires_at":null}]}`), nil
	}}

	grants, err := client.ListAccessGrants("vault-id")
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) != 2 {
		t.Fatalf("grant count = %d, want 2", len(grants))
	}
	if grants[0].ID != "user-grant" || grants[0].Principal.Email != "user@example.com" || grants[0].ExpiresAt == nil || *grants[0].ExpiresAt != "2026-12-01T15:00:00.123456Z" {
		t.Fatalf("unexpected user grant: %#v", grants[0])
	}
	if grants[1].ID != "service-account-grant" || grants[1].Principal.Name != "release-bot" || grants[1].ExpiresAt != nil {
		t.Fatalf("unexpected service-account grant: %#v", grants[1])
	}
}

func TestCreateAccessGrant(t *testing.T) {
	tests := []struct {
		name      string
		principal AccessGrantPrincipal
		expiresAt *string
		wantBody  string
	}{
		{
			name:      "expiring user",
			principal: AccessGrantPrincipal{Type: AccessGrantPrincipalTypeUser, Email: "user@example.com"},
			expiresAt: stringPointer("2026-12-01T15:00:00Z"),
			wantBody:  `{"principal_type":"user","email":"user@example.com","expires_at":"2026-12-01T15:00:00Z"}`,
		},
		{
			name:      "permanent service account",
			principal: AccessGrantPrincipal{Type: AccessGrantPrincipalTypeServiceAccount, Name: "release-bot"},
			wantBody:  `{"principal_type":"service_account","service_account":"release-bot","expires_at":null}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost {
					t.Fatalf("method = %q, want POST", req.Method)
				}
				if got, want := req.URL.String(), "/mint/api/vaults/vault-id/access_grants"; got != want {
					t.Fatalf("URL = %q, want %q", got, want)
				}
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatal(err)
				}
				if got := string(body); got != tt.wantBody {
					t.Fatalf("body = %s, want %s", got, tt.wantBody)
				}
				return jsonResponse(http.StatusCreated, `{"access_grant":{"id":"grant-id","vault":{"id":"vault-id","name":"production"},"principal":{"type":"user","id":"principal-id","email":"user@example.com"},"expires_at":null}}`), nil
			}}

			grant, err := client.CreateAccessGrant("vault-id", tt.principal, tt.expiresAt)
			if err != nil {
				t.Fatal(err)
			}
			if grant.ID != "grant-id" || grant.Vault.ID != "vault-id" {
				t.Fatalf("unexpected grant: %#v", grant)
			}
		})
	}
}

func TestGetAccessGrantNotFound(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusNotFound, ""), nil
	}}

	_, err := client.GetAccessGrant("vault-id", "grant-id")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestUpdateAccessGrant(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPatch {
			t.Fatalf("method = %q, want PATCH", req.Method)
		}
		if got, want := req.URL.String(), "/mint/api/vaults/vault-id/access_grants/grant-id"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(body), `{"expires_at":null}`; got != want {
			t.Fatalf("body = %s, want %s", got, want)
		}
		return jsonResponse(http.StatusOK, `{"access_grant":{"id":"grant-id","vault":{"id":"vault-id","name":"production"},"principal":{"type":"user","id":"user-id","email":"user@example.com"},"expires_at":null}}`), nil
	}}

	grant, err := client.UpdateAccessGrant("vault-id", "grant-id", nil)
	if err != nil {
		t.Fatal(err)
	}
	if grant.ExpiresAt != nil {
		t.Fatalf("expires_at = %v, want nil", grant.ExpiresAt)
	}
}

func TestDeleteAccessGrantIsIdempotent(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodDelete {
			t.Fatalf("method = %q, want DELETE", req.Method)
		}
		return jsonResponse(http.StatusNoContent, ""), nil
	}}

	if err := client.DeleteAccessGrant("vault-id", "missing-grant-id"); err != nil {
		t.Fatal(err)
	}
}

func TestFindAccessGrant(t *testing.T) {
	requests := 0
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		requests++
		switch req.URL.String() {
		case "/mint/api/vaults":
			return jsonResponse(http.StatusOK, `{"vaults":[{"id":"first-vault","name":"first"},{"id":"second-vault","name":"second"}]}`), nil
		case "/mint/api/vaults/first-vault/access_grants/grant-id":
			return jsonResponse(http.StatusNotFound, ""), nil
		case "/mint/api/vaults/second-vault/access_grants/grant-id":
			return jsonResponse(http.StatusOK, `{"access_grant":{"id":"grant-id","vault":{"id":"second-vault","name":"second"},"principal":{"type":"user","id":"user-id","email":"user@example.com"},"expires_at":null}}`), nil
		default:
			t.Fatalf("unexpected URL %q", req.URL.String())
			return nil, nil
		}
	}}

	grant, err := client.FindAccessGrant("grant-id")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 3 || grant.Vault.ID != "second-vault" {
		t.Fatalf("requests = %d, grant = %#v", requests, grant)
	}
}

func stringPointer(value string) *string {
	return &value
}
