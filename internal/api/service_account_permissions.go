package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

func (c Client) GetServiceAccountPermissions(id string) ([]Permission, error) {
	req, err := http.NewRequest(http.MethodGet, "/mint/api/service_accounts/"+url.PathEscape(id)+"/permissions", nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	return c.doServiceAccountPermissionsRequest(req)
}

func (c Client) ReplaceServiceAccountPermissions(id string, permissions []Permission) ([]Permission, error) {
	return c.replaceServiceAccountPermissions(id, permissions, "claim")
}

func (c Client) ClearServiceAccountPermissions(id string) error {
	_, err := c.replaceServiceAccountPermissions(id, []Permission{}, "release")
	return err
}

func (c Client) replaceServiceAccountPermissions(id string, permissions []Permission, management string) ([]Permission, error) {
	if permissions == nil {
		permissions = []Permission{}
	}
	encoded, err := json.Marshal(Permissions{Permissions: permissions})
	if err != nil {
		return nil, fmt.Errorf("unable to encode as JSON: %w", err)
	}
	req, err := http.NewRequest(http.MethodPut, "/mint/api/service_accounts/"+url.PathEscape(id)+"/permissions", bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TerraformManagementHeader, management)
	return c.doServiceAccountPermissionsRequest(req)
}

func (c Client) doServiceAccountPermissionsRequest(req *http.Request) ([]Permission, error) {
	resp, err := c.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, responseError(resp)
	}
	var result Permissions
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}
	return result.Permissions, nil
}
