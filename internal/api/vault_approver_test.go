package api

import (
	"errors"
	"io"
	"net/http"
	"testing"
)

func TestCreateVaultApprover(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if got, want := req.Method, http.MethodPost; got != want {
			t.Fatalf("method = %q, want %q", got, want)
		}
		if got, want := req.URL.String(), "/mint/api/vaults/vault-id/approvers"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		if got, want := req.Header.Get("Content-Type"), "application/json"; got != want {
			t.Fatalf("Content-Type = %q, want %q", got, want)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(body), `{"email":"reviewer@example.com"}`; got != want {
			t.Fatalf("body = %s, want %s", got, want)
		}

		return jsonResponse(http.StatusCreated, `{"approver":{"id":"identity-id","vault":{"id":"vault-id","name":"production"},"user":{"id":"user-id","email":"reviewer@example.com"}}}`), nil
	}}

	approver, err := client.CreateVaultApprover("vault-id", "reviewer@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if approver.ID != "identity-id" || approver.Vault.ID != "vault-id" || approver.User.ID != "user-id" || approver.User.Email != "reviewer@example.com" {
		t.Fatalf("unexpected approver: %#v", approver)
	}
}

func TestGetVaultApproverNotFound(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if got, want := req.URL.String(), "/mint/api/vaults/vault-id/approvers/identity-id"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		return jsonResponse(http.StatusNotFound, `{}`), nil
	}}

	_, err := client.GetVaultApprover("vault-id", "identity-id")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestDeleteVaultApprover(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if got, want := req.Method, http.MethodDelete; got != want {
			t.Fatalf("method = %q, want %q", got, want)
		}
		if got, want := req.URL.String(), "/mint/api/vaults/vault-id/approvers/identity-id"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		return jsonResponse(http.StatusNoContent, ``), nil
	}}

	if err := client.DeleteVaultApprover("vault-id", "identity-id"); err != nil {
		t.Fatal(err)
	}
}
