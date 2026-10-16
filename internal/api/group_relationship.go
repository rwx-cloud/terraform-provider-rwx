package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type GroupRelationship struct {
	ParentGroupID string `json:"parent_group_id"`
	ChildGroupID  string `json:"child_group_id"`
}

func (c Client) CreateGroupRelationship(parentGroupID, childGroupID string) (GroupRelationship, error) {
	return c.groupRelationshipRequest(http.MethodPost, parentGroupID, childGroupID)
}

func (c Client) GetGroupRelationship(parentGroupID, childGroupID string) (GroupRelationship, error) {
	return c.groupRelationshipRequest(http.MethodGet, parentGroupID, childGroupID)
}

func (c Client) DeleteGroupRelationship(parentGroupID, childGroupID string) error {
	_, err := c.groupRelationshipRequest(http.MethodDelete, parentGroupID, childGroupID)
	return err
}

func (c Client) groupRelationshipRequest(method, parentGroupID, childGroupID string) (GroupRelationship, error) {
	relationship := GroupRelationship{ParentGroupID: parentGroupID, ChildGroupID: childGroupID}
	endpoint := "/mint/api/group_relationships"
	var body bytes.Buffer
	if method == http.MethodGet {
		query := url.Values{"parent_group_id": {parentGroupID}, "child_group_id": {childGroupID}}
		endpoint += "?" + query.Encode()
	} else if err := json.NewEncoder(&body).Encode(relationship); err != nil {
		return GroupRelationship{}, fmt.Errorf("unable to encode as JSON: %w", err)
	}

	req, err := http.NewRequest(method, endpoint, &body)
	if err != nil {
		return GroupRelationship{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.RoundTrip(req)
	if err != nil {
		return GroupRelationship{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	expectedStatus := http.StatusOK
	switch method {
	case http.MethodPost:
		expectedStatus = http.StatusCreated
	case http.MethodDelete:
		expectedStatus = http.StatusNoContent
	}
	if resp.StatusCode == http.StatusNotFound {
		return GroupRelationship{}, ErrNotFound
	}
	if resp.StatusCode != expectedStatus {
		return GroupRelationship{}, responseError(resp)
	}
	if method == http.MethodDelete {
		return GroupRelationship{}, nil
	}

	var result struct {
		Relationship GroupRelationship `json:"group_relationship"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return GroupRelationship{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}
	return result.Relationship, nil
}
