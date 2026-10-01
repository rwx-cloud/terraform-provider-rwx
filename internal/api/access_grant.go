package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

const (
	AccessGrantPrincipalTypeUser           = "user"
	AccessGrantPrincipalTypeServiceAccount = "service_account"
)

type AccessGrantPrincipal struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Email string `json:"email,omitempty"`
	Name  string `json:"name,omitempty"`
}

type AccessGrant struct {
	ID        string               `json:"id"`
	Vault     Vault                `json:"vault"`
	Principal AccessGrantPrincipal `json:"principal"`
	ExpiresAt *string              `json:"expires_at"`
}

func (c Client) ListAccessGrants(vaultID string) ([]AccessGrant, error) {
	req, err := http.NewRequest(http.MethodGet, accessGrantsEndpoint(vaultID), nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create new HTTP request: %w", err)
	}

	resp, err := c.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, responseError(resp)
	}

	var result struct {
		AccessGrants []AccessGrant `json:"access_grants"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return result.AccessGrants, nil
}

func (c Client) CreateAccessGrant(vaultID string, principal AccessGrantPrincipal, expiresAt *string) (AccessGrant, error) {
	body := struct {
		PrincipalType  string  `json:"principal_type"`
		Email          string  `json:"email,omitempty"`
		ServiceAccount string  `json:"service_account,omitempty"`
		ExpiresAt      *string `json:"expires_at"`
	}{
		PrincipalType:  principal.Type,
		Email:          principal.Email,
		ServiceAccount: principal.Name,
		ExpiresAt:      expiresAt,
	}

	return c.writeAccessGrant(http.MethodPost, accessGrantsEndpoint(vaultID), body, http.StatusCreated)
}

func (c Client) GetAccessGrant(vaultID string, grantID string) (AccessGrant, error) {
	endpoint := accessGrantsEndpoint(vaultID) + "/" + url.PathEscape(grantID)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return AccessGrant{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}

	return c.doAccessGrantRequest(req, http.StatusOK)
}

func (c Client) UpdateAccessGrant(vaultID string, grantID string, expiresAt *string) (AccessGrant, error) {
	body := struct {
		ExpiresAt *string `json:"expires_at"`
	}{ExpiresAt: expiresAt}
	endpoint := accessGrantsEndpoint(vaultID) + "/" + url.PathEscape(grantID)

	return c.writeAccessGrant(http.MethodPatch, endpoint, body, http.StatusOK)
}

func (c Client) DeleteAccessGrant(vaultID string, grantID string) error {
	endpoint := accessGrantsEndpoint(vaultID) + "/" + url.PathEscape(grantID)
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

func (c Client) FindAccessGrant(grantID string) (AccessGrant, error) {
	vaults, err := c.ListVaults()
	if err != nil {
		return AccessGrant{}, err
	}

	for _, vault := range vaults {
		grant, err := c.GetAccessGrant(vault.ID, grantID)
		if err == nil {
			return grant, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return AccessGrant{}, err
		}
	}

	return AccessGrant{}, ErrNotFound
}

func (c Client) writeAccessGrant(method string, endpoint string, body any, expectedStatus int) (AccessGrant, error) {
	encodedBody, err := json.Marshal(body)
	if err != nil {
		return AccessGrant{}, fmt.Errorf("unable to encode as JSON: %w", err)
	}

	req, err := http.NewRequest(method, endpoint, bytes.NewBuffer(encodedBody))
	if err != nil {
		return AccessGrant{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doAccessGrantRequest(req, expectedStatus)
}

func (c Client) doAccessGrantRequest(req *http.Request, expectedStatus int) (AccessGrant, error) {
	resp, err := c.RoundTrip(req)
	if err != nil {
		return AccessGrant{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusNotFound {
		return AccessGrant{}, ErrNotFound
	}
	if resp.StatusCode != expectedStatus {
		return AccessGrant{}, responseError(resp)
	}

	var result struct {
		AccessGrant AccessGrant `json:"access_grant"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return AccessGrant{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return result.AccessGrant, nil
}

func accessGrantsEndpoint(vaultID string) string {
	return "/mint/api/vaults/" + url.PathEscape(vaultID) + "/access_grants"
}
