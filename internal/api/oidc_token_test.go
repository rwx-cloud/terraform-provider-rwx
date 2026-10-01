package api

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCreateOIDCToken(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("method = %q, want POST", req.Method)
		}
		if req.URL.String() != "/mint/api/vaults/oidc_tokens" {
			t.Fatalf("URL = %q, want /mint/api/vaults/oidc_tokens", req.URL.String())
		}
		if req.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", req.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(body), `{"vault_name":"my-vault","name":"aws","audience":"sts.amazonaws.com"}`; got != want {
			t.Fatalf("body = %s, want %s", got, want)
		}

		return jsonResponse(http.StatusCreated, `{
  "id":"token-id",
  "vault":{"id":"vault-id","name":"my-vault"},
  "name":"aws",
  "audience":"sts.amazonaws.com",
  "subject":"org:example:vault:my-vault",
  "expression":"${{ vaults.my-vault.oidc.aws }}",
  "documentation_url":"https://www.rwx.com/docs/oidc"
}`), nil
	}}

	token, err := client.CreateOIDCToken(VaultSelector{Name: "my-vault"}, OIDCToken{Name: "aws", Audience: "sts.amazonaws.com"})
	if err != nil {
		t.Fatal(err)
	}
	if token.ID != "token-id" || token.Vault.ID != "vault-id" || token.Vault.Name != "my-vault" {
		t.Fatalf("unexpected token identity: %#v", token)
	}
	if token.Subject != "org:example:vault:my-vault" || token.Expression != "${{ vaults.my-vault.oidc.aws }}" {
		t.Fatalf("unexpected computed fields: %#v", token)
	}
}

func TestGetOIDCTokenNotFound(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if got, want := req.URL.String(), "/mint/api/vaults/vault-id/oidc_tokens/token-id"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		return jsonResponse(http.StatusNotFound, `{}`), nil
	}}

	_, err := client.GetOIDCToken("vault-id", "token-id")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestUpdateOIDCTokenConflict(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPatch {
			t.Fatalf("method = %q, want PATCH", req.Method)
		}
		if got, want := req.URL.String(), "/mint/api/vaults/vault-id/oidc_tokens/token-id"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(body), `{"name":"new-name","audience":"new-audience"}`; got != want {
			t.Fatalf("body = %s, want %s", got, want)
		}
		return jsonResponse(http.StatusConflict, `{"errors":["Name must be unique"]}`), nil
	}}

	_, err := client.UpdateOIDCToken("vault-id", OIDCToken{ID: "token-id", Name: "new-name", Audience: "new-audience"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "Name must be unique") {
		t.Fatalf("error = %q, want API error message", err)
	}
}

func TestDeleteOIDCTokenNotFound(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodDelete {
			t.Fatalf("method = %q, want DELETE", req.Method)
		}
		if got, want := req.URL.String(), "/mint/api/vaults/vault-id/oidc_tokens/token-id"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		return jsonResponse(http.StatusNotFound, `{}`), nil
	}}

	err := client.DeleteOIDCToken("vault-id", "token-id")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestFindOIDCToken(t *testing.T) {
	requests := 0
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		requests++
		switch req.URL.String() {
		case "/mint/api/vaults":
			return jsonResponse(http.StatusOK, `{"vaults":[{"id":"first-vault","name":"first"},{"id":"second-vault","name":"second"}]}`), nil
		case "/mint/api/vaults/first-vault/oidc_tokens/token-id":
			return jsonResponse(http.StatusNotFound, `{}`), nil
		case "/mint/api/vaults/second-vault/oidc_tokens/token-id":
			return jsonResponse(http.StatusOK, `{"id":"token-id","vault":{"id":"second-vault","name":"second"},"name":"token","audience":"audience"}`), nil
		default:
			t.Fatalf("unexpected URL %q", req.URL.String())
			return nil, nil
		}
	}}

	token, err := client.FindOIDCToken("token-id")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 3 {
		t.Fatalf("requests = %d, want 3", requests)
	}
	if token.Vault.ID != "second-vault" || token.ID != "token-id" {
		t.Fatalf("unexpected token: %#v", token)
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
