package api

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGetVariableInVaultByID(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if got, want := req.URL.String(), "/mint/api/vaults/vars/region?vault_id=vault-id"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		return jsonResponse(http.StatusOK, `{"name":"region","value":"us-east-1"}`), nil
	}}

	variable, err := client.GetVariableInVault(VaultSelector{ID: "vault-id"}, Variable{Name: "region"})
	if err != nil {
		t.Fatal(err)
	}
	if variable.Value != "us-east-1" {
		t.Fatalf("value = %q, want us-east-1", variable.Value)
	}
}

func TestSetSecretInVaultByID(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(body), `{"secrets":[{"description":"deploy token","name":"token","secret":"value","version":0}],"vault_id":"vault-id"}`; got != want {
			t.Fatalf("body = %s, want %s", got, want)
		}
		return jsonResponse(http.StatusOK, `{"versions":{"token":2}}`), nil
	}}

	secret, err := client.SetSecretInVault(VaultSelector{ID: "vault-id"}, Secret{Name: "token", SecretValue: "value", Description: "deploy token"})
	if err != nil {
		t.Fatal(err)
	}
	if secret.Version != 2 {
		t.Fatalf("version = %d, want 2", secret.Version)
	}
}

func TestSetSecretInVaultBatchResults(t *testing.T) {
	for _, tc := range []struct {
		name, body, errorMessage string
		status                   int
		version                  int
	}{
		{"success", `{"versions":{"token":9}}`, "", 200, 9},
		{"partial batch failure", `{"versions":{"sibling":3},"error_messages":[{"message":"token could not be saved"}]}`, "token could not be saved", 400, 0},
		{"missing requested version", `{"versions":{"sibling":3}}`, "unable to infer secret version", 200, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost || req.URL.Path != "/mint/api/vaults/secrets" || req.Header.Get(TerraformManagementHeader) != "claim" {
					t.Fatalf("unexpected secret mutation: %s %s %v", req.Method, req.URL, req.Header)
				}
				return jsonResponse(tc.status, tc.body), nil
			}}
			secret, err := client.SetSecretInVault(VaultSelector{ID: "vault-two"}, Secret{Name: "token", SecretValue: "value"})
			if tc.errorMessage != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errorMessage) || secret != (Secret{}) {
					t.Fatalf("batch failure reported success: version=%d error=%v", secret.Version, err)
				}
			} else if err != nil || secret.Version != tc.version {
				t.Fatalf("secret version=%d, want %d; error=%v", secret.Version, tc.version, err)
			}
		})
	}
}
