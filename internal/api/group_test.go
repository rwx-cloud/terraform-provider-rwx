package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGroupLookupPagination(t *testing.T) {
	for _, denied := range []bool{false, true} {
		t.Run(fmt.Sprintf("denied=%t", denied), func(t *testing.T) {
			requests := 0
			client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
				requests++
				if requests > 2 {
					return nil, fmt.Errorf("lookup did not stop at the final page")
				}
				if req.URL.Query().Get("name") != "Engineering & IT" {
					t.Fatalf("lookup filter was lost: %s", req.URL)
				}
				if requests == 2 {
					if req.URL.Query().Get("cursor") != "next+/=&" {
						t.Fatalf("cursor was not preserved: %s", req.URL)
					}
					if denied {
						return jsonResponse(http.StatusForbidden, `{"error":"org:access required"}`), nil
					}
					return jsonResponse(http.StatusOK, `{"groups":[],"pagination":{"next_cursor":""}}`), nil
				}
				return jsonResponse(http.StatusOK, `{"groups":[{"id":"match-1","name":"Engineering & IT"}],"pagination":{"next_cursor":"next+/=&"}}`), nil
			}}
			group, err := client.FindGroupByName("Engineering & IT")
			if denied {
				if err == nil || errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "org:access required") || group.ID != "" {
					t.Fatalf("lookup returned a partial match or hid denial: ID=%q, error=%v", group.ID, err)
				}
			} else if err != nil || group.ID != "match-1" {
				t.Fatalf("lookup: ID=%q, error=%v; want match-1", group.ID, err)
			}
		})
	}
}

func TestGroupAPIFailures(t *testing.T) {
	for _, operation := range []struct {
		name string
		call func(Client) error
	}{
		{"create", func(c Client) error { _, err := c.CreateGroup(Group{Name: "Deployers"}); return err }},
		{"read", func(c Client) error { _, err := c.GetGroup("group-id"); return err }},
		{"update", func(c Client) error { _, err := c.UpdateGroup(Group{ID: "group-id", Name: "Renamed"}); return err }},
		{"delete", func(c Client) error { return c.DeleteGroup("group-id") }},
		{"lookup", func(c Client) error { _, err := c.FindGroupByName("Deployers"); return err }},
	} {
		for _, tc := range []struct {
			status        int
			body, message string
		}{
			{401, `{"error":"Unauthorized"}`, "Unauthorized"},
			{403, `{"error_messages":[{"message":"Requires group:manage"}]}`, "Requires group:manage"},
			{409, `{"errors":["System groups cannot be edited or deleted."]}`, "System groups cannot be edited or deleted."},
			{422, `{"errors":["Name has already been taken"]}`, "Name has already been taken"},
			{500, `not json`, "Unable to call RWX API"},
		} {
			t.Run(operation.name+"/"+http.StatusText(tc.status), func(t *testing.T) {
				client := Client{RoundTrip: func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: tc.status, Status: http.StatusText(tc.status), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
				}}
				err := operation.call(client)
				if err == nil || errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), tc.message) {
					t.Fatalf("got %v, want error containing %q, not absence", err, tc.message)
				}
			})
		}
	}
}

func TestGroupAPINotFound(t *testing.T) {
	client := Client{RoundTrip: func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}, nil
	}}
	if _, err := client.GetGroup("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetGroup: %v", err)
	}
	if err := client.DeleteGroup("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteGroup: %v", err)
	}
}
