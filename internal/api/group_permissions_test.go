package api

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGroupPermissionsErrors(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		for _, tc := range []struct {
			name     string
			status   int
			body     string
			notFound bool
			message  string
		}{
			{"missing", http.StatusNotFound, "", true, "not found"},
			{"unauthorized", http.StatusUnauthorized, `{"error":"Unauthorized"}`, false, "Unauthorized"},
			{"forbidden", http.StatusForbidden, `{"error_messages":[{"message":"Requires group:manage"}]}`, false, "Requires group:manage"},
			{"protected group", http.StatusUnprocessableEntity, `{"errors":["System groups cannot be edited"]}`, false, "System groups cannot be edited"},
			{"invalid response", http.StatusOK, `{`, false, "unable to decode JSON response"},
		} {
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
					if req.Method != method || req.URL.EscapedPath() != "/mint/api/groups/group%2Fone/permissions" {
						t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
					}
					if method == http.MethodPut {
						body, err := io.ReadAll(req.Body)
						if err != nil {
							t.Fatal(err)
						}
						if string(body) != `{"permissions":[]}` {
							t.Fatalf("clear body = %s", body)
						}
					}
					return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
				}}
				var err error
				if method == http.MethodGet {
					_, err = client.GetGroupPermissions("group/one")
				} else {
					_, err = client.ReplaceGroupPermissions("group/one", nil)
				}
				if err == nil || !strings.Contains(err.Error(), tc.message) || errors.Is(err, ErrNotFound) != tc.notFound {
					t.Fatalf("error = %v, want %q, not found = %v", err, tc.message, tc.notFound)
				}
			})
		}
	}
}
