package api

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestDirectoryGroupMappingReadErrors(t *testing.T) {
	for _, status := range []int{401, 403, 404, 409, 422, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
				return jsonResponse(status, `{"errors":["denied or unavailable"]}`), nil
			}}
			_, err := client.GetDirectoryGroupMapping(context.Background(), "source")
			if err == nil || errors.Is(err, ErrNotFound) != (status == 404) {
				t.Fatalf("status %d: %v", status, err)
			}
		})
	}
}

func TestDirectoryGroupMappingPollQuery(t *testing.T) {
	client := Client{RoundTrip: func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/mint/api/directory_group_mappings/source/reconciliation" || req.URL.Query().Get("reconciliation_id") != "write+1&2" {
			t.Fatalf("unexpected URL: %s", req.URL)
		}
		return jsonResponse(200, `{"reconciliation":{"reconciliation_id":"write+1&2","status":"failed","error":"IdP timed out"}}`), nil
	}}
	completion, err := client.GetDirectoryGroupMappingReconciliation(context.Background(), "source", "write+1&2")
	if err != nil || completion.Status != "failed" || completion.Error != "IdP timed out" {
		t.Fatalf("completion=%+v, err=%v", completion, err)
	}
}
