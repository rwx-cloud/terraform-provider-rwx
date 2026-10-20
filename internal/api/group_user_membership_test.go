package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestGroupUserMembershipRequests(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
				if req.Method != method || req.URL.Path != "/mint/api/group_user_memberships" {
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
				}
				if method == http.MethodGet {
					if req.URL.Query().Get("group_id") != "group & one" || req.URL.Query().Get("user_id") != "user/two" {
						t.Fatalf("unexpected query: %s", req.URL.RawQuery)
					}
				} else {
					var body map[string]string
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if len(body) != 2 || body["group_id"] != "group & one" || body["user_id"] != "user/two" || req.Header.Get("Content-Type") != "application/json" {
						t.Fatalf("unexpected body or content type: %#v, %s", body, req.Header.Get("Content-Type"))
					}
				}
				status := http.StatusOK
				if method == http.MethodPost {
					status = http.StatusCreated
				}
				if method == http.MethodDelete {
					return jsonResponse(http.StatusNoContent, ""), nil
				}
				return jsonResponse(status, `{"group_user_membership":{"group_id":"group & one","user_id":"user/two"}}`), nil
			}}
			var membership GroupUserMembership
			var err error
			switch method {
			case http.MethodGet:
				membership, err = client.GetGroupUserMembership("group & one", "user/two")
			case http.MethodPost:
				membership, err = client.CreateGroupUserMembership("group & one", "user/two")
			case http.MethodDelete:
				err = client.DeleteGroupUserMembership("group & one", "user/two")
			}
			if err != nil {
				t.Fatal(err)
			}
			if method != http.MethodDelete && (membership.GroupID != "group & one" || membership.UserID != "user/two") {
				t.Fatalf("unexpected membership: %#v", membership)
			}
		})
	}
}

func TestGroupUserMembershipErrors(t *testing.T) {
	for _, tc := range []struct {
		status  int
		body    string
		message string
	}{
		{400, "", "Bad Request"},
		{401, `{"error":"Unauthorized"}`, "Unauthorized"},
		{403, `{"error_messages":[{"message":"Requires group:manage"}],"required_permission":"group:manage"}`, "Requires group:manage"},
		{404, "", "not found"},
		{409, `{"errors":["That leaves the organization with no admin."]}`, "That leaves the organization with no admin."},
		{422, `{"errors":["Directory sync manages user membership in every RWX group."]}`, "Directory sync manages user membership in every RWX group."},
		{500, "", "Internal Server Error"},
	} {
		for _, operation := range []string{"get", "create", "delete"} {
			t.Run(operation+"/"+http.StatusText(tc.status), func(t *testing.T) {
				client := Client{RoundTrip: func(*http.Request) (*http.Response, error) {
					return jsonResponse(tc.status, tc.body), nil
				}}
				var err error
				switch operation {
				case "get":
					_, err = client.GetGroupUserMembership("group", "user")
				case "create":
					_, err = client.CreateGroupUserMembership("group", "user")
				case "delete":
					err = client.DeleteGroupUserMembership("group", "user")
				}
				if err == nil || !strings.Contains(err.Error(), tc.message) || errors.Is(err, ErrNotFound) != (tc.status == 404) {
					t.Fatalf("unexpected error: %v", err)
				}
			})
		}
	}
}
