package api

import (
	"errors"
	"io"
	"net/http"
	"testing"
)

func TestCreateVaultServiceAccountAttachment(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if got, want := req.Method, http.MethodPost; got != want {
			t.Fatalf("method = %q, want %q", got, want)
		}
		if got, want := req.URL.String(), "/mint/api/vaults/vault-id/service_account_attachments"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(body), `{"service_account":"deploy-bot"}`; got != want {
			t.Fatalf("body = %s, want %s", got, want)
		}

		return jsonResponse(http.StatusCreated, `{"service_account_attachment":{"id":"attachment-id","vault":{"id":"vault-id","name":"production"},"service_account":{"id":"account-id","name":"deploy-bot"},"created_at":"2026-10-01T12:34:56.123456Z","token":"must-not-be-stored"}}`), nil
	}}

	attachment, err := client.CreateVaultServiceAccountAttachment("vault-id", "deploy-bot")
	if err != nil {
		t.Fatal(err)
	}
	if attachment.ID != "attachment-id" || attachment.Vault.ID != "vault-id" || attachment.ServiceAccount.ID != "account-id" || attachment.ServiceAccount.Name != "deploy-bot" || attachment.CreatedAt != "2026-10-01T12:34:56.123456Z" {
		t.Fatalf("unexpected attachment: %#v", attachment)
	}
}

func TestGetVaultServiceAccountAttachmentNotFound(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if got, want := req.URL.String(), "/mint/api/vaults/vault-id/service_account_attachments/attachment-id"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		return jsonResponse(http.StatusNotFound, `{}`), nil
	}}

	_, err := client.GetVaultServiceAccountAttachment("vault-id", "attachment-id")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestDeleteVaultServiceAccountAttachment(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if got, want := req.Method, http.MethodDelete; got != want {
			t.Fatalf("method = %q, want %q", got, want)
		}
		if got, want := req.URL.String(), "/mint/api/vaults/vault-id/service_account_attachments/attachment-id"; got != want {
			t.Fatalf("URL = %q, want %q", got, want)
		}
		return jsonResponse(http.StatusNoContent, ``), nil
	}}

	if err := client.DeleteVaultServiceAccountAttachment("vault-id", "attachment-id"); err != nil {
		t.Fatal(err)
	}
}
