package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestUserLookup(t *testing.T) {
	for _, byEmail := range []bool{false, true} {
		t.Run(fmt.Sprintf("email=%t", byEmail), func(t *testing.T) {
			client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Fatalf("method = %s", req.Method)
				}
				if byEmail {
					if req.URL.Path != "/mint/api/organization_users/lookup" || req.URL.Query().Get("email") != "Engineer+ci&ops@Example.com" || len(req.URL.Query()) != 1 {
						t.Fatalf("unexpected email lookup: %s", req.URL)
					}
				} else if req.URL.EscapedPath() != "/mint/api/organization_users/user%2Fid" || req.URL.RawQuery != "" {
					t.Fatalf("unexpected ID lookup: %s", req.URL)
				}
				return jsonResponse(200, `{"user":{"id":"user-id","identity_id":"identity-id","email":"engineer+ci&ops@example.com"}}`), nil
			}}
			var user User
			var err error
			if byEmail {
				user, err = client.FindUserByEmail("Engineer+ci&ops@Example.com")
			} else {
				user, err = client.GetUser("user/id")
			}
			if err != nil || user.ID != "user-id" || user.IdentityID != "identity-id" || user.Email != "engineer+ci&ops@example.com" {
				t.Fatalf("user = %+v, error = %v", user, err)
			}
		})
	}
}

func TestUserLookupErrors(t *testing.T) {
	for _, test := range []struct {
		status     int
		body, want string
	}{
		{404, "", "not found"},
		{400, `{"errors":["email must be provided"]}`, "email must be provided"},
		{401, `{"error":"Unauthorized"}`, "Unauthorized"},
		{403, `{"error_messages":[{"message":"org:access required"}]}`, "org:access required"},
		{200, `{`, "unable to decode JSON response"},
	} {
		t.Run(fmt.Sprint(test.status), func(t *testing.T) {
			client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
				return jsonResponse(test.status, test.body), nil
			}}
			_, err := client.FindUserByEmail("user@example.com")
			if err == nil || !strings.Contains(err.Error(), test.want) || errors.Is(err, ErrNotFound) != (test.status == 404) {
				t.Fatalf("error = %v, want %q; only 404 is absence", err, test.want)
			}
		})
	}
}
