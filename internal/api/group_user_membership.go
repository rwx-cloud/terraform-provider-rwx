package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type GroupUserMembership struct {
	GroupID string `json:"group_id"`
	UserID  string `json:"user_id"`
}

func (c Client) CreateGroupUserMembership(groupID, userID string) (GroupUserMembership, error) {
	return c.groupUserMembershipRequest(http.MethodPost, groupID, userID)
}

func (c Client) GetGroupUserMembership(groupID, userID string) (GroupUserMembership, error) {
	return c.groupUserMembershipRequest(http.MethodGet, groupID, userID)
}

func (c Client) DeleteGroupUserMembership(groupID, userID string) error {
	_, err := c.groupUserMembershipRequest(http.MethodDelete, groupID, userID)
	return err
}

func (c Client) groupUserMembershipRequest(method, groupID, userID string) (GroupUserMembership, error) {
	membership := GroupUserMembership{GroupID: groupID, UserID: userID}
	endpoint := "/mint/api/group_user_memberships"
	var body bytes.Buffer
	if method == http.MethodGet {
		query := url.Values{"group_id": {groupID}, "user_id": {userID}}
		endpoint += "?" + query.Encode()
	} else if err := json.NewEncoder(&body).Encode(membership); err != nil {
		return GroupUserMembership{}, fmt.Errorf("unable to encode as JSON: %w", err)
	}
	req, err := http.NewRequest(method, endpoint, &body)
	if err != nil {
		return GroupUserMembership{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	if method == http.MethodPost {
		req.Header.Set(TerraformManagementHeader, "claim")
	}
	resp, err := c.RoundTrip(req)
	if err != nil {
		return GroupUserMembership{}, fmt.Errorf("HTTP request failed: %w", err)
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
		return GroupUserMembership{}, ErrNotFound
	}
	if resp.StatusCode != expectedStatus {
		return GroupUserMembership{}, responseError(resp)
	}
	if method == http.MethodDelete {
		return GroupUserMembership{}, nil
	}
	var result struct {
		Membership GroupUserMembership `json:"group_user_membership"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return GroupUserMembership{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}
	return result.Membership, nil
}
