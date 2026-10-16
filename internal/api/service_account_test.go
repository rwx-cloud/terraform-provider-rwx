package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestServiceAccountFailureClassification(t *testing.T) {
	for _, status := range []int{401, 403, 404, 409, 422, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
				return jsonResponse(status, `{"errors":["denied"]}`), nil
			}}
			_, readErr := client.GetServiceAccount("account-id")
			_, updateErr := client.UpdateServiceAccount(ServiceAccount{ID: "account-id", DisplayName: "New name"})
			deleteErr := client.DeleteServiceAccount("account-id")
			for _, err := range []error{readErr, updateErr, deleteErr} {
				if err == nil {
					t.Fatal("failure was ignored")
				}
				if errors.Is(err, ErrNotFound) != (status == 404) {
					t.Fatalf("status %d classified incorrectly: %v", status, err)
				}
			}
			if status == 409 && !errors.Is(updateErr, ErrConflict) {
				t.Fatalf("expected conflict: %v", updateErr)
			}
		})
	}
}

func TestServiceAccountLookupPagination(t *testing.T) {
	for _, selector := range []string{"name", "display_name"} {
		for _, denied := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/denied=%t", selector, denied), func(t *testing.T) {
				requests := 0
				client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
					requests++
					if requests > 2 {
						return nil, fmt.Errorf("lookup did not stop at the final page")
					}
					if req.URL.Query().Get(selector) != "Engineering & IT" {
						t.Fatalf("lookup filter was lost: %s", req.URL)
					}
					if requests == 2 {
						if req.URL.Query().Get("cursor") != "next+/=&" {
							t.Fatalf("cursor was not preserved: %s", req.URL)
						}
						if denied {
							return jsonResponse(http.StatusForbidden, `{"error":"org:access required"}`), nil
						}
						return jsonResponse(http.StatusOK, `{"service_accounts":[],"pagination":{"next_cursor":""}}`), nil
					}
					return jsonResponse(http.StatusOK, `{"service_accounts":[{"id":"match-1","name":"Engineering & IT"}],"pagination":{"next_cursor":"next+/=&"}}`), nil
				}}
				name, displayName := "Engineering & IT", ""
				if selector == "display_name" {
					name, displayName = "", name
				}
				account, err := client.FindServiceAccount(name, displayName)
				if denied {
					if err == nil || errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "org:access required") || account.ID != "" {
						t.Fatalf("lookup returned a partial match or hid denial: ID=%q, error=%v", account.ID, err)
					}
				} else if err != nil || account.ID != "match-1" {
					t.Fatalf("lookup: ID=%q, error=%v; want match-1", account.ID, err)
				}
			})
		}
	}
}
