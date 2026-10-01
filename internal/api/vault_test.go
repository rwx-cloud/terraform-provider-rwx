package api

import (
	"errors"
	"io"
	"net/http"
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
		if got, want := string(body), `{"name":"deploys","unlocked":true,"repository_permissions":[{"repository_slug":"rwx-cloud/cloud","branch_pattern":"main"}]}`; got != want {
			t.Fatalf("body = %s, want %s", got, want)
		}

		return jsonResponse(http.StatusCreated, `{"vault":{"id":"vault-id","name":"deploys","lock_status":"unlocked","repository_permissions":[{"repository_slug":"rwx-cloud/cloud","branch_pattern":"main"}],"oidc_subject":"org:example:vault:deploys"}}`), nil
	}}

	vault, err := client.CreateVault(Vault{
		Name:       "deploys",
		LockStatus: "unlocked",
		RepositoryPermissions: []VaultRepositoryPermission{
			{RepositorySlug: "rwx-cloud/cloud", BranchPattern: "main"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if vault.ID != "vault-id" || vault.OIDCSubject != "org:example:vault:deploys" {
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
		return jsonResponse(http.StatusOK, `{"vaults":[{"id":"first-id","name":"first"},{"id":"target-id","name":"target"}]}`), nil
	}}

	vault, err := client.FindVaultByName("target")
	if err != nil {
		t.Fatal(err)
	}
	if vault.ID != "target-id" {
		t.Fatalf("vault ID = %q, want target-id", vault.ID)
	}
}
