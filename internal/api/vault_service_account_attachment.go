package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type ServiceAccountIdentity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type VaultServiceAccountAttachment struct {
	ID             string                 `json:"id"`
	Vault          Vault                  `json:"vault"`
	ServiceAccount ServiceAccountIdentity `json:"service_account"`
	CreatedAt      string                 `json:"created_at"`
}

func (c Client) CreateVaultServiceAccountAttachment(vaultID string, serviceAccount string) (VaultServiceAccountAttachment, error) {
	body, err := json.Marshal(struct {
		ServiceAccount string `json:"service_account"`
	}{ServiceAccount: serviceAccount})
	if err != nil {
		return VaultServiceAccountAttachment{}, fmt.Errorf("unable to encode as JSON: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, vaultServiceAccountAttachmentsEndpoint(vaultID), bytes.NewBuffer(body))
	if err != nil {
		return VaultServiceAccountAttachment{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doVaultServiceAccountAttachmentRequest(req, http.StatusCreated)
}

func (c Client) GetVaultServiceAccountAttachment(vaultID string, attachmentID string) (VaultServiceAccountAttachment, error) {
	endpoint := vaultServiceAccountAttachmentsEndpoint(vaultID) + "/" + url.PathEscape(attachmentID)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return VaultServiceAccountAttachment{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}

	return c.doVaultServiceAccountAttachmentRequest(req, http.StatusOK)
}

func (c Client) DeleteVaultServiceAccountAttachment(vaultID string, attachmentID string) error {
	endpoint := vaultServiceAccountAttachmentsEndpoint(vaultID) + "/" + url.PathEscape(attachmentID)
	req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("unable to create new HTTP request: %w", err)
	}

	resp, err := c.RoundTrip(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusNoContent {
		return responseError(resp)
	}

	return nil
}

func (c Client) doVaultServiceAccountAttachmentRequest(req *http.Request, expectedStatus int) (VaultServiceAccountAttachment, error) {
	resp, err := c.RoundTrip(req)
	if err != nil {
		return VaultServiceAccountAttachment{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusNotFound {
		return VaultServiceAccountAttachment{}, ErrNotFound
	}
	if resp.StatusCode != expectedStatus {
		return VaultServiceAccountAttachment{}, responseError(resp)
	}

	var body struct {
		Attachment VaultServiceAccountAttachment `json:"service_account_attachment"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return VaultServiceAccountAttachment{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return body.Attachment, nil
}

func vaultServiceAccountAttachmentsEndpoint(vaultID string) string {
	return "/mint/api/vaults/" + url.PathEscape(vaultID) + "/service_account_attachments"
}
