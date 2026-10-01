package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type Vault struct {
	ID                    string                      `json:"id"`
	Name                  string                      `json:"name"`
	LockStatus            string                      `json:"lock_status"`
	RepositoryPermissions []VaultRepositoryPermission `json:"repository_permissions"`
	OIDCSubject           string                      `json:"oidc_subject"`
}

type VaultRepositoryPermission struct {
	RepositorySlug string `json:"repository_slug"`
	BranchPattern  string `json:"branch_pattern"`
}

type VaultSelector struct {
	ID   string
	Name string
}

func (c Client) CreateVault(vault Vault) (Vault, error) {
	body := vaultRequest(vault)

	return c.writeVault(http.MethodPost, "/mint/api/vaults", body, http.StatusCreated)
}

func (c Client) GetVault(vaultID string) (Vault, error) {
	endpoint := "/mint/api/vaults/" + url.PathEscape(vaultID)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return Vault{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}

	return c.doVaultRequest(req, http.StatusOK)
}

func (c Client) UpdateVault(vault Vault) (Vault, error) {
	body := vaultRequest(vault)
	endpoint := "/mint/api/vaults/" + url.PathEscape(vault.ID)

	return c.writeVault(http.MethodPatch, endpoint, body, http.StatusOK)
}

func (c Client) DeleteVault(vaultID string) error {
	endpoint := "/mint/api/vaults/" + url.PathEscape(vaultID)
	req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("unable to create new HTTP request: %w", err)
	}

	resp, err := c.RoundTrip(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode != http.StatusNoContent {
		return responseError(resp)
	}

	return nil
}

func (c Client) ListVaults() ([]Vault, error) {
	req, err := http.NewRequest(http.MethodGet, "/mint/api/vaults", nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create new HTTP request: %w", err)
	}

	resp, err := c.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, responseError(resp)
	}

	var response struct {
		Vaults []Vault `json:"vaults"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return response.Vaults, nil
}

func (c Client) FindVaultByName(name string) (Vault, error) {
	vaults, err := c.ListVaults()
	if err != nil {
		return Vault{}, err
	}

	for _, vault := range vaults {
		if vault.Name == name {
			return vault, nil
		}
	}

	return Vault{}, ErrNotFound
}

func vaultRequest(vault Vault) struct {
	Name                  string                      `json:"name"`
	Unlocked              bool                        `json:"unlocked"`
	RepositoryPermissions []VaultRepositoryPermission `json:"repository_permissions"`
} {
	return struct {
		Name                  string                      `json:"name"`
		Unlocked              bool                        `json:"unlocked"`
		RepositoryPermissions []VaultRepositoryPermission `json:"repository_permissions"`
	}{
		Name:                  vault.Name,
		Unlocked:              vault.LockStatus == "unlocked",
		RepositoryPermissions: vault.RepositoryPermissions,
	}
}

func (c Client) writeVault(method string, endpoint string, body any, expectedStatus int) (Vault, error) {
	encodedBody, err := json.Marshal(body)
	if err != nil {
		return Vault{}, fmt.Errorf("unable to encode as JSON: %w", err)
	}

	req, err := http.NewRequest(method, endpoint, bytes.NewBuffer(encodedBody))
	if err != nil {
		return Vault{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doVaultRequest(req, expectedStatus)
}

func (c Client) doVaultRequest(req *http.Request, expectedStatus int) (Vault, error) {
	resp, err := c.RoundTrip(req)
	if err != nil {
		return Vault{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusNotFound {
		return Vault{}, ErrNotFound
	}
	if resp.StatusCode == http.StatusConflict {
		return Vault{}, fmt.Errorf("%w: %s", ErrConflict, responseErrorMessage(resp))
	}
	if resp.StatusCode != expectedStatus {
		return Vault{}, responseError(resp)
	}

	var response struct {
		Vault Vault `json:"vault"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return Vault{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return response.Vault, nil
}

func vaultSelectorQuery(selector VaultSelector) string {
	query := url.Values{}
	if selector.ID != "" {
		query.Set("vault_id", selector.ID)
	} else {
		query.Set("vault_name", selector.Name)
	}

	return query.Encode()
}

func vaultSelectorBody(selector VaultSelector) (vaultID string, vaultName string) {
	if selector.ID != "" {
		return selector.ID, ""
	}

	return "", selector.Name
}
