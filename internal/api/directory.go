package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type Directory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c Client) GetDirectory(id string) (Directory, error) {
	req, err := http.NewRequest(http.MethodGet, "/mint/api/directories/"+url.PathEscape(id), nil)
	if err != nil {
		return Directory{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	resp, err := c.RoundTrip(req)
	if err != nil {
		return Directory{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return Directory{}, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return Directory{}, responseError(resp)
	}
	var response struct {
		Directory Directory `json:"directory"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return Directory{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}
	return response.Directory, nil
}

func (c Client) ListDirectories(name string) ([]Directory, error) {
	query := url.Values{"name": {name}}
	var directories []Directory
	for {
		req, err := http.NewRequest(http.MethodGet, "/mint/api/directories?"+query.Encode(), nil)
		if err != nil {
			return nil, fmt.Errorf("unable to create new HTTP request: %w", err)
		}
		resp, err := c.RoundTrip(req)
		if err != nil {
			return nil, fmt.Errorf("HTTP request failed: %w", err)
		}
		var response struct {
			Directories []Directory `json:"directories"`
			Pagination  struct {
				NextCursor string `json:"next_cursor"`
			} `json:"pagination"`
		}
		if resp.StatusCode != http.StatusOK {
			err := responseError(resp)
			_ = resp.Body.Close()
			return nil, err
		}
		err = json.NewDecoder(resp.Body).Decode(&response)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("unable to decode JSON response: %w", err)
		}
		directories = append(directories, response.Directories...)
		if response.Pagination.NextCursor == "" {
			return directories, nil
		}
		query.Set("cursor", response.Pagination.NextCursor)
	}
}

func (c Client) FindDirectoryByName(name string) (Directory, error) {
	directories, err := c.ListDirectories(name)
	if err != nil {
		return Directory{}, err
	}
	if len(directories) == 0 {
		return Directory{}, ErrNotFound
	}
	if len(directories) > 1 {
		return Directory{}, fmt.Errorf("multiple directories named %q; use an ID instead", name)
	}
	return directories[0], nil
}
