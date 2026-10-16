package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type Permission struct {
	Key        string  `json:"key"`
	ResourceID *string `json:"resource_id,omitempty"`
}

type Permissions struct {
	Permissions []Permission `json:"permissions"`
}

type PermissionCatalog struct {
	Permissions            []CatalogPermission `json:"permissions"`
	UnremovablePermissions []string            `json:"unremovable_permissions"`
}

type CatalogPermission struct {
	Key                   string  `json:"key"`
	SupportsResourceScope bool    `json:"supports_resource_scope"`
	ScopeResource         *string `json:"scope_resource"`
	Grantable             struct {
		Unscoped    bool     `json:"unscoped"`
		ResourceIDs []string `json:"resource_ids"`
	} `json:"grantable"`
}

func (c Client) GetPermissionCatalog(target url.Values) (PermissionCatalog, error) {
	req, err := http.NewRequest(http.MethodGet, "/mint/api/permission_catalog?"+target.Encode(), nil)
	if err != nil {
		return PermissionCatalog{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	resp, err := c.RoundTrip(req)
	if err != nil {
		return PermissionCatalog{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return PermissionCatalog{}, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return PermissionCatalog{}, responseError(resp)
	}
	var catalog PermissionCatalog
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		return PermissionCatalog{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}
	return catalog, nil
}
