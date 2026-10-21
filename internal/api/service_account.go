package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type ServiceAccount struct {
	ID          string  `json:"id"`
	IdentityID  string  `json:"identity_id"`
	Name        string  `json:"name"`
	DisplayName string  `json:"display_name"`
	Description *string `json:"description"`
}

func (c Client) CreateServiceAccount(account ServiceAccount) (ServiceAccount, error) {
	return c.writeServiceAccount(http.MethodPost, "/mint/api/service_accounts", account, http.StatusCreated)
}

func (c Client) UpdateServiceAccount(account ServiceAccount) (ServiceAccount, error) {
	return c.writeServiceAccount(http.MethodPatch, "/mint/api/service_accounts/"+url.PathEscape(account.ID), account, http.StatusOK)
}

func (c Client) GetServiceAccount(id string) (ServiceAccount, error) {
	req, err := http.NewRequest(http.MethodGet, "/mint/api/service_accounts/"+url.PathEscape(id), nil)
	if err != nil {
		return ServiceAccount{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	return c.doServiceAccountRequest(req, http.StatusOK)
}

func (c Client) DeleteServiceAccount(id string) error {
	req, err := http.NewRequest(http.MethodDelete, "/mint/api/service_accounts/"+url.PathEscape(id), nil)
	if err != nil {
		return fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	resp, err := c.RoundTrip(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode != http.StatusNoContent {
		return responseError(resp)
	}
	return nil
}

func (c Client) ListServiceAccounts(key string) ([]ServiceAccount, error) {
	query := url.Values{"limit": {"100"}}
	if key != "" {
		query.Set("key", key)
	}
	var accounts []ServiceAccount
	for {
		req, err := http.NewRequest(http.MethodGet, "/mint/api/service_accounts?"+query.Encode(), nil)
		if err != nil {
			return nil, fmt.Errorf("unable to create new HTTP request: %w", err)
		}
		resp, err := c.RoundTrip(req)
		if err != nil {
			return nil, fmt.Errorf("HTTP request failed: %w", err)
		}
		var page struct {
			ServiceAccounts []ServiceAccount `json:"service_accounts"`
			Pagination      struct {
				NextCursor string `json:"next_cursor"`
			} `json:"pagination"`
		}
		if resp.StatusCode != http.StatusOK {
			err = responseError(resp)
			_ = resp.Body.Close()
			return nil, err
		}
		err = json.NewDecoder(resp.Body).Decode(&page)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("unable to decode JSON response: %w", err)
		}
		accounts = append(accounts, page.ServiceAccounts...)
		if page.Pagination.NextCursor == "" {
			return accounts, nil
		}
		query.Set("cursor", page.Pagination.NextCursor)
	}
}

func (c Client) FindServiceAccount(key string) (ServiceAccount, error) {
	accounts, err := c.ListServiceAccounts(key)
	if err != nil {
		return ServiceAccount{}, err
	}
	if len(accounts) == 0 {
		return ServiceAccount{}, ErrNotFound
	}
	if len(accounts) > 1 {
		return ServiceAccount{}, fmt.Errorf("multiple active service accounts matched key %q", key)
	}
	return accounts[0], nil
}

func (c Client) writeServiceAccount(method, endpoint string, account ServiceAccount, status int) (ServiceAccount, error) {
	body := struct {
		DisplayName string  `json:"display_name"`
		Description *string `json:"description"`
	}{
		DisplayName: account.DisplayName,
		Description: account.Description,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return ServiceAccount{}, fmt.Errorf("unable to encode as JSON: %w", err)
	}
	req, err := http.NewRequest(method, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return ServiceAccount{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.doServiceAccountRequest(req, status)
}

func (c Client) doServiceAccountRequest(req *http.Request, status int) (ServiceAccount, error) {
	resp, err := c.RoundTrip(req)
	if err != nil {
		return ServiceAccount{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return ServiceAccount{}, ErrNotFound
	}
	if resp.StatusCode == http.StatusConflict {
		return ServiceAccount{}, fmt.Errorf("%w: %s", ErrConflict, responseErrorMessage(resp))
	}
	if resp.StatusCode != status {
		return ServiceAccount{}, responseError(resp)
	}
	var response struct {
		ServiceAccount ServiceAccount `json:"service_account"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return ServiceAccount{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}
	return response.ServiceAccount, nil
}
