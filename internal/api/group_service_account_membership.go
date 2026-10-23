package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type GroupServiceAccountMembership struct {
	GroupID          string `json:"group_id"`
	ServiceAccountID string `json:"service_account_id"`
}

func (c Client) CreateGroupServiceAccountMembership(groupID, serviceAccountID string) (GroupServiceAccountMembership, error) {
	return c.groupServiceAccountMembershipRequest(http.MethodPost, groupID, serviceAccountID)
}

func (c Client) GetGroupServiceAccountMembership(groupID, serviceAccountID string) (GroupServiceAccountMembership, error) {
	return c.groupServiceAccountMembershipRequest(http.MethodGet, groupID, serviceAccountID)
}

func (c Client) DeleteGroupServiceAccountMembership(groupID, serviceAccountID string) error {
	_, err := c.groupServiceAccountMembershipRequest(http.MethodDelete, groupID, serviceAccountID)
	return err
}

func (c Client) groupServiceAccountMembershipRequest(method, groupID, serviceAccountID string) (GroupServiceAccountMembership, error) {
	membership := GroupServiceAccountMembership{GroupID: groupID, ServiceAccountID: serviceAccountID}
	endpoint := "/mint/api/group_service_account_memberships"
	var body bytes.Buffer
	if method == http.MethodGet {
		query := url.Values{"group_id": {groupID}, "service_account_id": {serviceAccountID}}
		endpoint += "?" + query.Encode()
	} else if err := json.NewEncoder(&body).Encode(membership); err != nil {
		return GroupServiceAccountMembership{}, fmt.Errorf("unable to encode as JSON: %w", err)
	}
	req, err := http.NewRequest(method, endpoint, &body)
	if err != nil {
		return GroupServiceAccountMembership{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	if method == http.MethodPost {
		req.Header.Set(TerraformManagementHeader, "claim")
	}
	resp, err := c.RoundTrip(req)
	if err != nil {
		return GroupServiceAccountMembership{}, fmt.Errorf("HTTP request failed: %w", err)
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
		return GroupServiceAccountMembership{}, ErrNotFound
	}
	if resp.StatusCode != expectedStatus {
		return GroupServiceAccountMembership{}, responseError(resp)
	}
	if method == http.MethodDelete {
		return GroupServiceAccountMembership{}, nil
	}
	var result struct {
		Membership GroupServiceAccountMembership `json:"group_service_account_membership"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return GroupServiceAccountMembership{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}
	return result.Membership, nil
}
