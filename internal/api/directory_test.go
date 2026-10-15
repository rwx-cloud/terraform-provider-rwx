package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestDirectoryReadErrors(t *testing.T) {
	for _, byName := range []bool{false, true} {
		for _, test := range []struct {
			name   string
			status int
			body   string
			want   string
		}{
			{"unauthorized", 401, `{"error":"Unauthorized"}`, "Unauthorized"},
			{"forbidden", 403, `{"error":"org:access required"}`, "org:access required"},
			{"malformed", 200, `{`, "unable to decode JSON response"},
		} {
			t.Run(test.name+map[bool]string{true: " name", false: " ID"}[byName], func(t *testing.T) {
				client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
					return jsonResponse(test.status, test.body), nil
				}}
				var err error
				if byName {
					_, err = client.FindDirectoryByName("Engineering")
				} else {
					_, err = client.GetDirectory("local-id")
				}
				if err == nil || errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("error = %v, want %q (not absence)", err, test.want)
				}
			})
		}
	}
}

func TestDirectoryLookupPagination(t *testing.T) {
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
					return jsonResponse(http.StatusOK, `{"directories":[],"pagination":{"next_cursor":""}}`), nil
				}
				return jsonResponse(http.StatusOK, `{"directories":[{"id":"match-1","name":"Engineering & IT"}],"pagination":{"next_cursor":"next+/=&"}}`), nil
			}}
			directory, err := client.FindDirectoryByName("Engineering & IT")
			if denied {
				if err == nil || errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "org:access required") || directory.ID != "" {
					t.Fatalf("lookup returned a partial match or hid denial: ID=%q, error=%v", directory.ID, err)
				}
			} else if err != nil || directory.ID != "match-1" {
				t.Fatalf("lookup: ID=%q, error=%v; want match-1", directory.ID, err)
			}
		})
	}
}
