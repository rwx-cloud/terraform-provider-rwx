package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

func (c Client) GetGroupPermissions(id string) ([]Permission, error) {
	req, err := http.NewRequest(http.MethodGet, "/mint/api/groups/"+url.PathEscape(id)+"/permissions", nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	return c.doGroupPermissionsRequest(req)
}

func (c Client) ReplaceGroupPermissions(id string, permissions []Permission) ([]Permission, error) {
	if permissions == nil {
		permissions = []Permission{}
	}
	encoded, err := json.Marshal(Permissions{Permissions: permissions})
	if err != nil {
		return nil, fmt.Errorf("unable to encode as JSON: %w", err)
	}
	req, err := http.NewRequest(http.MethodPut, "/mint/api/groups/"+url.PathEscape(id)+"/permissions", bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.doGroupPermissionsRequest(req)
}

func (c Client) doGroupPermissionsRequest(req *http.Request) ([]Permission, error) {
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
