package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type Group struct {
	ID          string  `json:"id,omitempty"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

func (c Client) CreateGroup(group Group) (Group, error) {
	return c.writeGroup(http.MethodPost, "/mint/api/groups", group, http.StatusCreated)
}

func (c Client) UpdateGroup(group Group) (Group, error) {
	return c.writeGroup(http.MethodPatch, "/mint/api/groups/"+url.PathEscape(group.ID), group, http.StatusOK)
}

func (c Client) GetGroup(id string) (Group, error) {
	req, err := http.NewRequest(http.MethodGet, "/mint/api/groups/"+url.PathEscape(id), nil)
	if err != nil {
		return Group{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	return c.doGroupRequest(req, http.StatusOK)
}

func (c Client) DeleteGroup(id string) error {
	req, err := http.NewRequest(http.MethodDelete, "/mint/api/groups/"+url.PathEscape(id), nil)
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

func (c Client) FindGroupByName(name string) (Group, error) {
	query := url.Values{"name": {name}, "limit": {"100"}}
	var matches []Group
	for {
		req, err := http.NewRequest(http.MethodGet, "/mint/api/groups?"+query.Encode(), nil)
		if err != nil {
			return Group{}, fmt.Errorf("unable to create new HTTP request: %w", err)
		}
		resp, err := c.RoundTrip(req)
		if err != nil {
			return Group{}, fmt.Errorf("HTTP request failed: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			err := responseError(resp)
			_ = resp.Body.Close()
			return Group{}, err
		}
		var page struct {
			Groups     []Group `json:"groups"`
			Pagination struct {
				NextCursor string `json:"next_cursor"`
			} `json:"pagination"`
		}
		err = json.NewDecoder(resp.Body).Decode(&page)
		_ = resp.Body.Close()
		if err != nil {
			return Group{}, fmt.Errorf("unable to decode JSON response: %w", err)
		}
		for _, group := range page.Groups {
			if group.Name == name {
				matches = append(matches, group)
			}
		}
		if page.Pagination.NextCursor == "" {
			break
		}
		query.Set("cursor", page.Pagination.NextCursor)
	}
	if len(matches) == 0 {
		return Group{}, ErrNotFound
	}
	if len(matches) > 1 {
		return Group{}, fmt.Errorf("multiple groups named %q; use an ID instead", name)
	}
	return matches[0], nil
}

func (c Client) writeGroup(method, endpoint string, group Group, status int) (Group, error) {
	body := struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}{
		Name:        group.Name,
		Description: group.Description,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return Group{}, fmt.Errorf("unable to encode as JSON: %w", err)
	}
	req, err := http.NewRequest(method, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return Group{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TerraformManagementHeader, "claim")
	return c.doGroupRequest(req, status)
}

func (c Client) doGroupRequest(req *http.Request, status int) (Group, error) {
	resp, err := c.RoundTrip(req)
	if err != nil {
		return Group{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return Group{}, ErrNotFound
	}
	if resp.StatusCode == http.StatusConflict {
		return Group{}, fmt.Errorf("%w: %s", ErrConflict, responseErrorMessage(resp))
	}
	if resp.StatusCode != status {
		return Group{}, responseError(resp)
	}
	var result struct {
		Group Group `json:"group"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Group{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}
	return result.Group, nil
}
