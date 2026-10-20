package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type User struct {
	ID         string `json:"id"`
	IdentityID string `json:"identity_id"`
	Email      string `json:"email"`
}

func (c Client) GetUser(id string) (User, error) {
	return c.getUser("/mint/api/organization_users/" + url.PathEscape(id))
}

func (c Client) FindUserByEmail(email string) (User, error) {
	return c.getUser("/mint/api/organization_users/lookup?" + url.Values{"email": {email}}.Encode())
}

func (c Client) getUser(endpoint string) (User, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return User{}, fmt.Errorf("unable to create new HTTP request: %w", err)
	}
	resp, err := c.RoundTrip(req)
	if err != nil {
		return User{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return User{}, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return User{}, responseError(resp)
	}
	var response struct {
		User User `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return User{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}
	return response.User, nil
}
