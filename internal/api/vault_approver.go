package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type UserIdentity struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type VaultApprover struct {
	ID    string       `json:"id"`
	Vault Vault        `json:"vault"`
	User  UserIdentity `json:"user"`
}

func (c Client) CreateVaultApprover(vaultID string, email string) (VaultApprover, error) {
	body, err := json.Marshal(struct {
		Email string `json:"email"`
	}{Email: email})
	if err != nil {
		return VaultApprover{}, fmt.Errorf("unable to encode as JSON: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, vaultApproversEndpoint(vaultID), bytes.NewBuffer(body))
	if err != nil {
		return VaultApprover{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doVaultApproverRequest(req, http.StatusCreated)
}

func (c Client) GetVaultApprover(vaultID string, approverID string) (VaultApprover, error) {
	endpoint := vaultApproversEndpoint(vaultID) + "/" + url.PathEscape(approverID)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return VaultApprover{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}

	return c.doVaultApproverRequest(req, http.StatusOK)
}

func (c Client) DeleteVaultApprover(vaultID string, approverID string) error {
	endpoint := vaultApproversEndpoint(vaultID) + "/" + url.PathEscape(approverID)
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

func (c Client) doVaultApproverRequest(req *http.Request, expectedStatus int) (VaultApprover, error) {
	resp, err := c.RoundTrip(req)
	if err != nil {
		return VaultApprover{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusNotFound {
		return VaultApprover{}, ErrNotFound
	}
	if resp.StatusCode != expectedStatus {
		return VaultApprover{}, responseError(resp)
	}

	var body struct {
		Approver VaultApprover `json:"approver"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return VaultApprover{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return body.Approver, nil
}

func vaultApproversEndpoint(vaultID string) string {
	return "/mint/api/vaults/" + url.PathEscape(vaultID) + "/approvers"
}
