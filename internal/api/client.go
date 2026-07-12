package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Client is an API Client for RWX
type Client struct {
	RoundTrip func(*http.Request) (*http.Response, error)
}

func NewClient(cfg Config) (Client, error) {
	if err := cfg.Validate(); err != nil {
		return Client{}, fmt.Errorf("validation failed: %w", err)
	}

	roundTrip := func(req *http.Request) (*http.Response, error) {
		if req.URL.Scheme == "" {
			req.URL.Scheme = "https"
		}
		if req.URL.Host == "" {
			req.URL.Host = cfg.Host
		}

		req.Header.Set("User-Agent", fmt.Sprintf("terraform-provider-rwx/%s", cfg.Version))
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.AccessToken))

		return http.DefaultClient.Do(req)
	}

	return Client{roundTrip}, nil
}

func (c Client) DeleteSecretInVault(vault string, secret Secret) error {
	endpoint := "/mint/api/vaults/secrets"

	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/%s?vault_name=%s", endpoint, secret.Name, vault), nil)
	if err != nil {
		return fmt.Errorf("unable to create new HTTP request: %w", err)
	}

	resp, err := c.RoundTrip(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != 200 && resp.StatusCode != 404 {
		msg := extractErrorMessage(resp.Body)
		if msg == "" {
			msg = fmt.Sprintf("Unable to call RWX API - %s", resp.Status)
		}

		return errors.New(msg)
	}

	return nil
}

func (c Client) DeleteVariableInVault(vault string, variable Variable) error {
	endpoint := "/mint/api/vaults/vars"

	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/%s?vault_name=%s", endpoint, variable.Name, vault), nil)
	if err != nil {
		return fmt.Errorf("unable to create new HTTP request: %w", err)
	}

	resp, err := c.RoundTrip(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != 200 && resp.StatusCode != 404 {
		msg := extractErrorMessage(resp.Body)
		if msg == "" {
			msg = fmt.Sprintf("Unable to call RWX API - %s", resp.Status)
		}

		return errors.New(msg)
	}

	return nil
}

func (c Client) GetSecretMetadataInVault(vault string, secret Secret) (Secret, error) {
	endpoint := "/mint/api/vaults/secrets"

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s?vault_name=%s", endpoint, secret.Name, vault), nil)
	if err != nil {
		return Secret{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}

	resp, err := c.RoundTrip(req)
	if err != nil {
		return Secret{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != 200 {
		if resp.StatusCode == 404 {
			return Secret{}, ErrNotFound
		}

		msg := extractErrorMessage(resp.Body)
		if msg == "" {
			msg = fmt.Sprintf("Unable to call RWX API - %s", resp.Status)
		}

		return Secret{}, errors.New(msg)
	}

	if err := json.NewDecoder(resp.Body).Decode(&secret); err != nil {
		return Secret{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return secret, nil
}

func (c Client) GetVariableInVault(vault string, variable Variable) (Variable, error) {
	endpoint := "/mint/api/vaults/vars"

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s?vault_name=%s", endpoint, variable.Name, vault), nil)
	if err != nil {
		return Variable{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}

	resp, err := c.RoundTrip(req)
	if err != nil {
		return Variable{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != 200 {
		if resp.StatusCode == 404 {
			return Variable{}, ErrNotFound
		}

		msg := extractErrorMessage(resp.Body)
		if msg == "" {
			msg = fmt.Sprintf("Unable to call RWX API - %s", resp.Status)
		}

		return Variable{}, errors.New(msg)
	}

	if err := json.NewDecoder(resp.Body).Decode(&variable); err != nil {
		return Variable{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return variable, nil
}

func (c Client) SetSecretInVault(vault string, secret Secret) (Secret, error) {
	endpoint := "/mint/api/vaults/secrets"

	requestBody := struct {
		Secrets   []Secret `json:"secrets"`
		VaultName string   `json:"vault_name"`
	}{
		Secrets:   []Secret{secret},
		VaultName: vault,
	}

	encodedBody, err := json.Marshal(requestBody)
	if err != nil {
		return Secret{}, fmt.Errorf("unable to encode as JSON: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(encodedBody))
	if err != nil {
		return Secret{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.RoundTrip(req)
	if err != nil {
		return Secret{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != 200 {
		if resp.StatusCode == 404 {
			return Secret{}, ErrNotFound
		}

		msg := extractErrorMessage(resp.Body)
		if msg == "" {
			msg = fmt.Sprintf("Unable to call RWX API - %s", resp.Status)
		}

		return Secret{}, errors.New(msg)
	}

	var response = struct {
		Versions map[string]int `json:"versions"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return Secret{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	var ok bool
	if secret.Version, ok = response.Versions[secret.Name]; !ok {
		return Secret{}, fmt.Errorf("unable to infer secret version from response")
	}

	return secret, nil
}

func (c Client) SetVariableInVault(vault string, variable Variable) (Variable, error) {
	endpoint := "/mint/api/vaults/vars"

	requestBody := struct {
		Var       Variable `json:"var"`
		VaultName string   `json:"vault_name"`
	}{
		Var:       variable,
		VaultName: vault,
	}

	encodedBody, err := json.Marshal(requestBody)
	if err != nil {
		return Variable{}, fmt.Errorf("unable to encode as JSON: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(encodedBody))
	if err != nil {
		return Variable{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.RoundTrip(req)
	if err != nil {
		return Variable{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != 200 {
		if resp.StatusCode == 404 {
			return Variable{}, ErrNotFound
		}

		msg := extractErrorMessage(resp.Body)
		if msg == "" {
			msg = fmt.Sprintf("Unable to call RWX API - %s", resp.Status)
		}

		return Variable{}, errors.New(msg)
	}

	return variable, nil
}
