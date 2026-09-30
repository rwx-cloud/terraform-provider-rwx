package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

type OIDCToken struct {
	ID         string `json:"id"`
	Vault      Vault  `json:"vault"`
	Name       string `json:"name"`
	Audience   string `json:"audience"`
	Subject    string `json:"subject"`
	Expression string `json:"expression"`
}

type Vault struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c Client) CreateOIDCToken(vaultName string, token OIDCToken) (OIDCToken, error) {
	body := struct {
		VaultName string `json:"vault_name"`
		Name      string `json:"name"`
		Audience  string `json:"audience"`
	}{
		VaultName: vaultName,
		Name:      token.Name,
		Audience:  token.Audience,
	}

	return c.writeOIDCToken(http.MethodPost, "/mint/api/vaults/oidc_tokens", body, http.StatusCreated)
}

func (c Client) GetOIDCToken(vaultID string, tokenID string) (OIDCToken, error) {
	endpoint := oidcTokensEndpoint(vaultID) + "/" + url.PathEscape(tokenID)

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return OIDCToken{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}

	return c.doOIDCTokenRequest(req, http.StatusOK)
}

func (c Client) UpdateOIDCToken(vaultID string, token OIDCToken) (OIDCToken, error) {
	body := struct {
		Name     string `json:"name"`
		Audience string `json:"audience"`
	}{
		Name:     token.Name,
		Audience: token.Audience,
	}
	endpoint := oidcTokensEndpoint(vaultID) + "/" + url.PathEscape(token.ID)

	return c.writeOIDCToken(http.MethodPatch, endpoint, body, http.StatusOK)
}

func (c Client) DeleteOIDCToken(vaultID string, tokenID string) error {
	endpoint := oidcTokensEndpoint(vaultID) + "/" + url.PathEscape(tokenID)

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
	if resp.StatusCode != http.StatusOK {
		return responseError(resp)
	}

	return nil
}

func (c Client) FindOIDCToken(tokenID string) (OIDCToken, error) {
	vaults, err := c.listVaults()
	if err != nil {
		return OIDCToken{}, err
	}

	for _, vault := range vaults {
		token, err := c.GetOIDCToken(vault.ID, tokenID)
		if err == nil {
			return token, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return OIDCToken{}, err
		}
	}

	return OIDCToken{}, ErrNotFound
}

func (c Client) listVaults() ([]Vault, error) {
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

func (c Client) writeOIDCToken(method string, endpoint string, body any, expectedStatus int) (OIDCToken, error) {
	encodedBody, err := json.Marshal(body)
	if err != nil {
		return OIDCToken{}, fmt.Errorf("unable to encode as JSON: %w", err)
	}

	req, err := http.NewRequest(method, endpoint, bytes.NewBuffer(encodedBody))
	if err != nil {
		return OIDCToken{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doOIDCTokenRequest(req, expectedStatus)
}

func (c Client) doOIDCTokenRequest(req *http.Request, expectedStatus int) (OIDCToken, error) {
	resp, err := c.RoundTrip(req)
	if err != nil {
		return OIDCToken{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusNotFound {
		return OIDCToken{}, ErrNotFound
	}
	if resp.StatusCode == http.StatusConflict {
		return OIDCToken{}, fmt.Errorf("%w: %s", ErrConflict, responseErrorMessage(resp))
	}
	if resp.StatusCode != expectedStatus {
		return OIDCToken{}, responseError(resp)
	}

	var token OIDCToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return OIDCToken{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return token, nil
}

func oidcTokensEndpoint(vaultID string) string {
	return "/mint/api/vaults/" + url.PathEscape(vaultID) + "/oidc_tokens"
}

func responseError(resp *http.Response) error {
	message := responseErrorMessage(resp)
	if message == "" {
		message = fmt.Sprintf("Unable to call RWX API - %s", resp.Status)
	}

	return errors.New(message)
}

func responseErrorMessage(resp *http.Response) string {
	return extractErrorMessage(resp.Body)
}
