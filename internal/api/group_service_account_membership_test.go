package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestGroupServiceAccountMembershipRequests(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
				if req.Method != method || req.URL.Path != "/mint/api/group_service_account_memberships" {
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
				}
				if method == http.MethodGet {
					if req.URL.Query().Get("group_id") != "group & one" || req.URL.Query().Get("service_account_id") != "account/two" {
						t.Fatalf("unexpected query: %s", req.URL.RawQuery)
					}
				} else {
					var body map[string]string
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if len(body) != 2 || body["group_id"] != "group & one" || body["service_account_id"] != "account/two" || req.Header.Get("Content-Type") != "application/json" {
						t.Fatalf("unexpected body or content type: %#v", body)
					}
				}
				status := http.StatusOK
				switch method {
				case http.MethodPost:
					status = http.StatusCreated
				case http.MethodDelete:
					return jsonResponse(http.StatusNoContent, ""), nil
				}
				return jsonResponse(status, `{"group_service_account_membership":{"group_id":"group & one","service_account_id":"account/two"}}`), nil
			}}
			var membership GroupServiceAccountMembership
			var err error
			switch method {
			case http.MethodGet:
				membership, err = client.GetGroupServiceAccountMembership("group & one", "account/two")
			case http.MethodPost:
				membership, err = client.CreateGroupServiceAccountMembership("group & one", "account/two")
			case http.MethodDelete:
				err = client.DeleteGroupServiceAccountMembership("group & one", "account/two")
			}
			if err != nil {
				t.Fatal(err)
			}
			if method != http.MethodDelete && (membership.GroupID != "group & one" || membership.ServiceAccountID != "account/two") {
				t.Fatalf("unexpected membership: %#v", membership)
			}
		})
	}
}

func TestGroupServiceAccountMembershipErrors(t *testing.T) {
	for _, tc := range []struct {
		status        int
		body, message string
	}{
		{400, "", "Bad Request"},
		{401, `{"error":"Unauthorized"}`, "Unauthorized"},
		{403, `{"error_messages":[{"message":"You do not have permission to perform this action. It requires the \"group:manage\" permission."}],"required_permission":"group:manage"}`, `requires the "group:manage" permission`},
		{404, "", "not found"},
		{409, `{"errors":["That leaves the organization with no admin."]}`, "That leaves the organization with no admin."},
		{422, `{"errors":["Unable to add service account"]}`, "Unable to add service account"},
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
					_, err = client.GetGroupServiceAccountMembership("group", "account")
				case "create":
					_, err = client.CreateGroupServiceAccountMembership("group", "account")
				case "delete":
					err = client.DeleteGroupServiceAccountMembership("group", "account")
				}
				if err == nil || !strings.Contains(err.Error(), tc.message) || errors.Is(err, ErrNotFound) != (tc.status == 404) {
					t.Fatalf("unexpected error: %v", err)
				}
			})
		}
	}
}
