package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestGroupRelationshipRequests(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
				if req.Method != method || req.URL.Path != "/mint/api/group_relationships" {
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
				}
				if method == http.MethodGet {
					if req.URL.Query().Get("parent_group_id") != "parent & one" || req.URL.Query().Get("child_group_id") != "child/two" {
						t.Fatalf("unexpected query: %s", req.URL.RawQuery)
					}
				} else {
					var body map[string]string
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if len(body) != 2 || body["parent_group_id"] != "parent & one" || body["child_group_id"] != "child/two" || req.Header.Get("Content-Type") != "application/json" {
						t.Fatalf("unexpected body or content type: %#v, %s", body, req.Header.Get("Content-Type"))
					}
				}
				status := http.StatusOK
				switch method {
				case http.MethodPost:
					status = http.StatusCreated
				case http.MethodDelete:
					return jsonResponse(http.StatusNoContent, ""), nil
				}
				return jsonResponse(status, `{"group_relationship":{"parent_group_id":"parent & one","child_group_id":"child/two"}}`), nil
			}}
			var relationship GroupRelationship
			var err error
			switch method {
			case http.MethodGet:
				relationship, err = client.GetGroupRelationship("parent & one", "child/two")
			case http.MethodPost:
				relationship, err = client.CreateGroupRelationship("parent & one", "child/two")
			case http.MethodDelete:
				err = client.DeleteGroupRelationship("parent & one", "child/two")
			}
			if err != nil {
				t.Fatal(err)
			}
			if method != http.MethodDelete && (relationship.ParentGroupID != "parent & one" || relationship.ChildGroupID != "child/two") {
				t.Fatalf("unexpected relationship: %#v", relationship)
			}
		})
	}
}

func TestGroupRelationshipErrors(t *testing.T) {
	for _, tc := range []struct {
		status  int
		body    string
		message string
	}{
		{401, `{"error":"Unauthorized"}`, "Unauthorized"},
		{403, `{"error_messages":[{"message":"Requires group:manage"}]}`, "Requires group:manage"},
		{404, "", "not found"},
		{409, `{"errors":["That leaves the organization with no admin."]}`, "That leaves the organization with no admin."},
		{422, `{"errors":["Child is already above the parent in the group hierarchy"]}`, "Child is already above the parent"},
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
					_, err = client.GetGroupRelationship("parent", "child")
				case "create":
					_, err = client.CreateGroupRelationship("parent", "child")
				case "delete":
					err = client.DeleteGroupRelationship("parent", "child")
				}
				if err == nil || !strings.Contains(err.Error(), tc.message) || errors.Is(err, ErrNotFound) != (tc.status == 404) {
					t.Fatalf("unexpected error: %v", err)
				}
			})
		}
	}
}
