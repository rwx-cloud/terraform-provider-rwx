package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestFindDirectoryGroupByName(t *testing.T) {
	for _, tt := range []struct {
		name      string
		pages     []string
		wantID    string
		wantError string
	}{
		{"later page", []string{`{"directory_groups":[],"pagination":{"next_cursor":"opaque +/="}}`, `{"directory_groups":[{"id":"group-2","directory_id":"dir &1","name":"Engineering +%"}],"pagination":{"next_cursor":null}}`}, "group-2", ""},
		{"ambiguous across pages", []string{`{"directory_groups":[{"id":"group-1","directory_id":"dir &1","name":"Engineering +%"}],"pagination":{"next_cursor":"opaque +/="}}`, `{"directory_groups":[{"id":"group-2","directory_id":"dir &1","name":"Engineering +%"}],"pagination":{"next_cursor":null}}`}, "", "multiple directory groups"},
		{"missing", []string{`{"directory_groups":[],"pagination":{"next_cursor":null}}`}, "", "not found"},
		{"case sensitive", []string{`{"directory_groups":[{"id":"group-1","directory_id":"dir &1","name":"engineering +%"}]}`}, "", "not found"},
		{"wrong directory", []string{`{"directory_groups":[{"id":"group-1","directory_id":"other","name":"Engineering +%"}]}`}, "", "does not match"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			page := 0
			client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
				q := req.URL.Query()
				if req.Method != http.MethodGet || req.URL.Path != "/mint/api/directory_groups" || q.Get("directory_id") != "dir &1" || q.Get("name") != "Engineering +%" {
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
				}
				if page > 0 && q.Get("cursor") != "opaque +/=" {
					t.Fatalf("cursor = %q", q.Get("cursor"))
				}
				if page >= len(tt.pages) {
					t.Fatal("unexpected extra page")
				}
				body := tt.pages[page]
				page++
				return jsonResponse(http.StatusOK, body), nil
			}}
			group, err := client.FindDirectoryGroupByName("dir &1", "Engineering +%")
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("error = %v, want %q", err, tt.wantError)
				}
			} else if err != nil || group.ID != tt.wantID {
				t.Fatalf("group = %#v, error = %v", group, err)
			}
		})
	}
}

func TestGetDirectoryGroup(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.EscapedPath() != "/mint/api/directory_groups/group%2F1" || req.URL.Query().Get("directory_id") != "directory-1" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
		}
		return jsonResponse(http.StatusOK, `{"directory_group":{"id":"group/1","directory_id":"directory-1","name":"Renamed"}}`), nil
	}}
	group, err := client.GetDirectoryGroup("directory-1", "group/1")
	if err != nil || group.ID != "group/1" || group.Name != "Renamed" {
		t.Fatalf("group = %#v, error = %v", group, err)
	}
}

func TestDirectoryGroupLookupPagination(t *testing.T) {
	for _, denied := range []bool{false, true} {
		t.Run(fmt.Sprintf("denied=%t", denied), func(t *testing.T) {
			requests := 0
			client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
				requests++
				if requests > 2 {
					return nil, fmt.Errorf("lookup did not stop at the final page")
				}
				if req.URL.Query().Get("directory_id") != "directory-1" || req.URL.Query().Get("name") != "Engineering & IT" {
					t.Fatalf("lookup filters were lost: %s", req.URL)
				}
				if requests == 2 {
					if req.URL.Query().Get("cursor") != "next+/=&" {
						t.Fatalf("cursor was not preserved: %s", req.URL)
					}
					if denied {
						return jsonResponse(http.StatusForbidden, `{"error":"org:access required"}`), nil
					}
					return jsonResponse(http.StatusOK, `{"directory_groups":[],"pagination":{"next_cursor":""}}`), nil
				}
				return jsonResponse(http.StatusOK, `{"directory_groups":[{"id":"match-1","directory_id":"directory-1","name":"Engineering & IT"}],"pagination":{"next_cursor":"next+/=&"}}`), nil
			}}
			group, err := client.FindDirectoryGroupByName("directory-1", "Engineering & IT")
			if denied {
				if err == nil || errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "org:access required") || group.ID != "" {
					t.Fatalf("lookup returned a partial match or hid denial: group=%#v, error=%v", group, err)
				}
			} else if err != nil || group.ID != "match-1" {
				t.Fatalf("lookup: group=%#v, error=%v; want match-1", group, err)
			}
		})
	}
}

func TestDirectoryGroupErrors(t *testing.T) {
	for _, status := range []int{401, 403, 404, 500} {
		for _, byID := range []bool{true, false} {
			client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
				return jsonResponse(status, `{"error":"denied"}`), nil
			}}
			var err error
			if byID {
				_, err = client.GetDirectoryGroup("dir", "group")
			} else {
				_, err = client.FindDirectoryGroupByName("dir", "Engineering")
			}
			if err == nil || errors.Is(err, ErrNotFound) != (status == 404) {
				t.Fatalf("status %d, byID %t: error = %v", status, byID, err)
			}
		}
	}
}
