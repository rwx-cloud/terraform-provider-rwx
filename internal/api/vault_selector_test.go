package api

import (
	"io"
	"net/http"
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
