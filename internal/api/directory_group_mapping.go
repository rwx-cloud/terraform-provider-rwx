package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type DirectoryGroupMapping struct {
	ID               string `json:"id"`
	DirectoryID      string `json:"directory_id"`
	DirectoryGroupID string `json:"directory_group_id"`
	GroupID          string `json:"group_id"`
}

type DirectoryGroupMappingReconciliation struct {
	ID     string `json:"reconciliation_id"`
	Status string `json:"status"`
	Error  string `json:"error"`
}

type DirectoryGroupMappingResponse struct {
	Mapping        *DirectoryGroupMapping              `json:"directory_group_mapping"`
	Reconciliation DirectoryGroupMappingReconciliation `json:"reconciliation"`
}

func (c Client) CreateDirectoryGroupMapping(ctx context.Context, mapping DirectoryGroupMapping) (DirectoryGroupMappingResponse, error) {
	body := struct {
		DirectoryID      string `json:"directory_id"`
		DirectoryGroupID string `json:"directory_group_id"`
		GroupID          string `json:"group_id"`
	}{mapping.DirectoryID, mapping.DirectoryGroupID, mapping.GroupID}
	return c.directoryGroupMappingRequest(ctx, http.MethodPost, directoryGroupMappingsEndpoint, body)
}

func (c Client) GetDirectoryGroupMapping(ctx context.Context, id string) (DirectoryGroupMappingResponse, error) {
	return c.directoryGroupMappingRequest(ctx, http.MethodGet, directoryGroupMappingEndpoint(id), nil)
}

func (c Client) UpdateDirectoryGroupMapping(ctx context.Context, id, groupID string) (DirectoryGroupMappingResponse, error) {
	return c.directoryGroupMappingRequest(ctx, http.MethodPatch, directoryGroupMappingEndpoint(id), struct {
		GroupID string `json:"group_id"`
	}{groupID})
}

func (c Client) DeleteDirectoryGroupMapping(ctx context.Context, id string) (DirectoryGroupMappingResponse, error) {
	return c.directoryGroupMappingRequest(ctx, http.MethodDelete, directoryGroupMappingEndpoint(id), nil)
}

func (c Client) GetDirectoryGroupMappingReconciliation(ctx context.Context, id, reconciliationID string) (DirectoryGroupMappingReconciliation, error) {
	endpoint := directoryGroupMappingEndpoint(id) + "/reconciliation"
	if reconciliationID != "" {
		endpoint += "?" + url.Values{"reconciliation_id": {reconciliationID}}.Encode()
	}
	response, err := c.directoryGroupMappingRequest(ctx, http.MethodGet, endpoint, nil)
	return response.Reconciliation, err
}

const directoryGroupMappingsEndpoint = "/mint/api/directory_group_mappings"

func directoryGroupMappingEndpoint(id string) string {
	return directoryGroupMappingsEndpoint + "/" + url.PathEscape(id)
}

func (c Client) directoryGroupMappingRequest(ctx context.Context, method, endpoint string, body any) (DirectoryGroupMappingResponse, error) {
	var encoded bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&encoded).Encode(body); err != nil {
			return DirectoryGroupMappingResponse{}, fmt.Errorf("unable to encode JSON: %w", err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, &encoded)
	if err != nil {
		return DirectoryGroupMappingResponse{}, fmt.Errorf("unable to create HTTP request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.RoundTrip(req)
	if err != nil {
		return DirectoryGroupMappingResponse{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return DirectoryGroupMappingResponse{}, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return DirectoryGroupMappingResponse{}, responseError(resp)
	}
	var result DirectoryGroupMappingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return result, fmt.Errorf("unable to decode JSON response: %w", err)
	}
	return result, nil
}
