package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type DirectoryGroup struct {
	ID          string `json:"id"`
	DirectoryID string `json:"directory_id"`
	Name        string `json:"name"`
}

func (c Client) GetDirectoryGroup(directoryID, id string) (DirectoryGroup, error) {
	query := url.Values{"directory_id": {directoryID}}
	req, err := http.NewRequest(http.MethodGet, "/mint/api/directory_groups/"+url.PathEscape(id)+"?"+query.Encode(), nil)
	if err != nil {
		return DirectoryGroup{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	var response struct {
		Group DirectoryGroup `json:"directory_group"`
	}
	if err := c.readDirectoryGroups(req, &response); err != nil {
		return DirectoryGroup{}, err
	}
	if response.Group.ID != id || response.Group.DirectoryID != directoryID {
		return DirectoryGroup{}, fmt.Errorf("directory group response does not match the requested ID and directory")
	}
	return response.Group, nil
}

func (c Client) FindDirectoryGroupByName(directoryID, name string) (DirectoryGroup, error) {
	query := url.Values{"directory_id": {directoryID}, "name": {name}}
	var matches []DirectoryGroup
	for {
		req, err := http.NewRequest(http.MethodGet, "/mint/api/directory_groups?"+query.Encode(), nil)
		if err != nil {
			return DirectoryGroup{}, fmt.Errorf("unable to create new HTTP request: %w", err)
		}
		var response struct {
			Groups     []DirectoryGroup `json:"directory_groups"`
			Pagination struct {
				NextCursor string `json:"next_cursor"`
			} `json:"pagination"`
		}
		if err := c.readDirectoryGroups(req, &response); err != nil {
			return DirectoryGroup{}, err
		}
		for _, group := range response.Groups {
			if group.DirectoryID != directoryID {
				return DirectoryGroup{}, fmt.Errorf("directory group response does not match the requested directory")
			}
			if group.Name == name {
				matches = append(matches, group)
			}
		}
		if response.Pagination.NextCursor == "" {
			break
		}
		query.Set("cursor", response.Pagination.NextCursor)
	}
	if len(matches) == 0 {
		return DirectoryGroup{}, ErrNotFound
	}
	if len(matches) > 1 {
		return DirectoryGroup{}, fmt.Errorf("multiple directory groups named %q in directory %q; use id instead", name, directoryID)
	}
	return matches[0], nil
}

func (c Client) readDirectoryGroups(req *http.Request, result any) error {
	resp, err := c.RoundTrip(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return responseError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("unable to decode JSON response: %w", err)
	}
	return nil
}
