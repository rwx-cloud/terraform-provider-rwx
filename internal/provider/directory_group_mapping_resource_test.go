package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"
)

type mappingBackend struct {
	mu                sync.Mutex
	mappings          map[string]api.DirectoryGroupMapping
	completions       map[string]api.DirectoryGroupMappingReconciliation
	enabled           bool
	outcome           string
	denied            bool
	queueFailure      bool
	recoverOnDelete   bool
	recoverOnSave     bool
	disableAfterWrite bool
	writes            int
	polls             int
	deletes           int
}

func mappingTestBackend(t *testing.T, enabled bool) *mappingBackend {
	t.Helper()
	b := &mappingBackend{mappings: map[string]api.DirectoryGroupMapping{}, completions: map[string]api.DirectoryGroupMappingReconciliation{}, enabled: enabled, outcome: "succeeded"}
	previous := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: b}
	t.Cleanup(func() { http.DefaultClient = previous })
	return b
}

func (b *mappingBackend) RoundTrip(req *http.Request) (*http.Response, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	respond := func(status int, value any) (*http.Response, error) {
		body, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	}
	if req.Header.Get("Authorization") != "Bearer service-account-test-token" {
		return respond(401, map[string]string{"error": "Unauthorized"})
	}
	if b.denied {
		return respond(403, map[string][]string{"errors": {"group:manage denied"}})
	}
	base := "/mint/api/directory_group_mappings"
	id := strings.TrimPrefix(req.URL.Path, base+"/")
	if strings.HasSuffix(id, "/reconciliation") {
		b.polls++
		id = strings.TrimSuffix(id, "/reconciliation")
		completion := b.completions[id]
		if wanted := req.URL.Query().Get("reconciliation_id"); wanted != "" && wanted != completion.ID {
			completion = api.DirectoryGroupMappingReconciliation{Status: "unknown", Error: "superseded"}
		} else {
			completion.Status = b.outcome
			if completion.Status == "failed" {
				completion.Error = "Directory sync is not enabled"
			}
			b.completions[id] = completion
		}
		return respond(200, api.DirectoryGroupMappingResponse{Reconciliation: completion})
	}
	if req.Method == http.MethodGet {
		mapping, ok := b.mappings[id]
		if !ok {
			return respond(404, map[string][]string{"errors": {"not found"}})
		}
		return respond(200, api.DirectoryGroupMappingResponse{Mapping: &mapping, Reconciliation: b.completions[id]})
	}
	var mapping api.DirectoryGroupMapping
	switch req.Method {
	case http.MethodPost, http.MethodPatch:
		if err := json.NewDecoder(req.Body).Decode(&mapping); err != nil {
			return nil, err
		}
		if req.Method == http.MethodPost {
			id = mapping.DirectoryGroupID
			mapping.ID = id
		} else {
			saved, ok := b.mappings[id]
			if !ok {
				return respond(404, map[string][]string{"errors": {"not found"}})
			}
			mapping = api.DirectoryGroupMapping{ID: id, DirectoryID: saved.DirectoryID, DirectoryGroupID: saved.DirectoryGroupID, GroupID: mapping.GroupID}
		}
		b.mappings[id] = mapping
		if b.recoverOnSave {
			b.outcome = "succeeded"
			b.queueFailure = false
		}
	case http.MethodDelete:
		delete(b.mappings, id)
		b.deletes++
		if b.recoverOnDelete {
			b.outcome = "succeeded"
			b.queueFailure = false
		}
	default:
		return nil, fmt.Errorf("unexpected request %s %s", req.Method, req.URL)
	}
	b.writes++
	completion := api.DirectoryGroupMappingReconciliation{Status: "succeeded"}
	status := 200
	if b.enabled {
		status = 202
		completion.ID = fmt.Sprintf("write-%d", b.writes)
		completion.Status = "pending"
	}
	if b.disableAfterWrite {
		b.enabled = false
		b.outcome = "failed"
		b.disableAfterWrite = false
	}
	b.completions[id] = completion
	if b.queueFailure {
		completion.Status = "failed"
		b.completions[id] = completion
		return respond(422, map[string][]string{"errors": {"Mapping saved but reconciliation could not be queued. Retry the request."}})
	}
	result := api.DirectoryGroupMappingResponse{Reconciliation: completion}
	if req.Method != http.MethodDelete {
		result.Mapping = &mapping
	}
	return respond(status, result)
}

func mappingConfig(source, target, timeout string) string {
	return fmt.Sprintf(`
provider "rwx" {
  host = "mapping.test"
  access_token = "service-account-test-token"
}
resource "rwx_directory_group_mapping" "test" {
  directory_id = "directory-1"
  directory_group_id = %q
  group_id = %q
  reconciliation_timeout = %q
}
`, source, target, timeout)
}

func TestDirectoryGroupMappingLifecycle(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%t", enabled), func(t *testing.T) {
			b := mappingTestBackend(t, enabled)
			initial := mappingConfig("source-a", "target-a", "20m")
			updated := mappingConfig("source-a", "target-b", "5s")
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{Config: initial, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("rwx_directory_group_mapping.test", "id", "source-a"), resource.TestCheckResourceAttr("rwx_directory_group_mapping.test", "reconciliation_status", "succeeded"))},
					{Config: initial, PlanOnly: true},
					{ResourceName: "rwx_directory_group_mapping.test", ImportState: true, ImportStateVerify: true},
					{Config: initial, PlanOnly: true},
					{Config: updated, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("rwx_directory_group_mapping.test", "id", "source-a"), resource.TestCheckResourceAttr("rwx_directory_group_mapping.test", "group_id", "target-b"))},
					{Config: updated, PlanOnly: true},
					{PreConfig: func() {
						b.mu.Lock()
						defer b.mu.Unlock()
						m := b.mappings["source-a"]
						m.GroupID = "ui-target"
						b.mappings["source-a"] = m
					}, Config: updated, PlanOnly: true, ExpectNonEmptyPlan: true},
					{Config: updated},
					{PreConfig: func() { b.mu.Lock(); defer b.mu.Unlock(); delete(b.mappings, "source-a") }, Config: updated},
					{Config: mappingConfig("source-b", "target-b", "5s"), Check: resource.TestCheckResourceAttr("rwx_directory_group_mapping.test", "id", "source-b")},
				},
			})
			b.mu.Lock()
			defer b.mu.Unlock()
			if !enabled && b.polls != 0 {
				t.Fatalf("draft performed %d polls", b.polls)
			}
			if enabled && b.polls == 0 {
				t.Fatal("enabled writes did not poll")
			}
			if len(b.mappings) != 0 {
				t.Fatalf("mappings remain: %v", b.mappings)
			}
		})
	}
}

func TestDirectoryGroupMappingUpdateRecovery(t *testing.T) {
	for _, outcome := range []string{"failed", "pending", "unknown", "queue-failure", "disabled"} {
		t.Run(outcome, func(t *testing.T) {
			b := mappingTestBackend(t, true)
			initial := mappingConfig("source-a", "target-a", "5s")
			updated := mappingConfig("source-a", "target-b", "50ms")
			errorPattern := `deadline exceeded`
			if outcome == "failed" || outcome == "disabled" {
				errorPattern = `Directory sync is not enabled`
			}
			if outcome == "unknown" {
				errorPattern = `reconciliation unknown`
			}
			if outcome == "queue-failure" {
				errorPattern = `could not be queued`
			}
			if outcome != "pending" {
				updated = mappingConfig("source-a", "target-b", "5s")
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{Config: initial},
					{PreConfig: func() {
						b.mu.Lock()
						defer b.mu.Unlock()
						switch outcome {
						case "queue-failure":
							b.queueFailure = true
						case "disabled":
							b.disableAfterWrite = true
						default:
							b.outcome = outcome
						}
					}, Config: updated, ExpectError: regexp.MustCompile(errorPattern)},
					{PreConfig: func() {
						b.mu.Lock()
						defer b.mu.Unlock()
						if b.mappings["source-a"].GroupID != "target-b" {
							t.Fatal("acknowledged target lost")
						}
						b.outcome = "succeeded"
						b.queueFailure = false
					}, Config: mappingConfig("source-a", "target-b", "5s"), Check: resource.TestCheckResourceAttr("rwx_directory_group_mapping.test", "reconciliation_status", "succeeded")},
					{Config: mappingConfig("source-a", "target-b", "5s"), PlanOnly: true},
				},
			})
		})
	}
}

func TestDirectoryGroupMappingDeniedRead(t *testing.T) {
	b := mappingTestBackend(t, false)
	config := mappingConfig("source-a", "target-a", "5s")
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
		{Config: config},
		{PreConfig: func() { b.mu.Lock(); defer b.mu.Unlock(); b.denied = true }, Config: config, PlanOnly: true, ExpectError: regexp.MustCompile("group:manage denied")},
		{PreConfig: func() { b.mu.Lock(); defer b.mu.Unlock(); b.denied = false }, Config: config, PlanOnly: true},
	}})
}

func TestDirectoryGroupMappingDestroyRecovery(t *testing.T) {
	for _, outcome := range []string{"failed", "pending", "queue-failure", "restore"} {
		t.Run(outcome, func(t *testing.T) {
			b := mappingTestBackend(t, true)
			timeout, pattern := "5s", "Directory sync is not enabled"
			if outcome == "pending" {
				timeout, pattern = "50ms", "deadline exceeded"
			}
			if outcome == "queue-failure" {
				pattern = "could not be queued"
			}
			config := mappingConfig("source-a", "target-a", timeout)
			resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
				{Config: config},
				{PreConfig: func() {
					b.mu.Lock()
					defer b.mu.Unlock()
					b.outcome = "failed"
					if outcome == "pending" {
						b.outcome = "pending"
					}
					b.queueFailure = outcome == "queue-failure"
				}, Config: config, Destroy: true, ExpectError: regexp.MustCompile(pattern)},
				{PreConfig: func() { b.mu.Lock(); defer b.mu.Unlock(); b.recoverOnDelete = true; b.recoverOnSave = true }, Config: config, Destroy: outcome != "restore"},
			}})
			b.mu.Lock()
			defer b.mu.Unlock()
			if b.deletes < 2 {
				t.Fatalf("failed removal was forgotten: %d DELETEs", b.deletes)
			}
		})
	}
}

func TestDirectoryGroupMappingCreateRecovery(t *testing.T) {
	for _, outcome := range []string{"failed", "pending", "queue-failure"} {
		t.Run(outcome, func(t *testing.T) {
			b := mappingTestBackend(t, true)
			b.outcome = outcome
			b.queueFailure = outcome == "queue-failure"
			timeout := "5s"
			pattern := "Directory sync is not enabled"
			if outcome == "pending" {
				timeout = "50ms"
				pattern = "deadline exceeded"
			}
			if b.queueFailure {
				pattern = "could not be queued"
			}
			resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
				{Config: mappingConfig("source-a", "target-a", timeout), ExpectError: regexp.MustCompile(pattern)},
				{PreConfig: func() {
					b.mu.Lock()
					defer b.mu.Unlock()
					b.outcome = "succeeded"
					b.queueFailure = false
					b.recoverOnDelete = true
				}, Config: mappingConfig("source-a", "target-a", "5s"), Check: resource.TestCheckResourceAttr("rwx_directory_group_mapping.test", "id", "source-a")},
				{Config: mappingConfig("source-a", "target-a", "5s"), PlanOnly: true},
			}})
			b.mu.Lock()
			defer b.mu.Unlock()
			if b.deletes < 2 {
				t.Fatalf("failed create did not retain identity for replacement: %d DELETEs", b.deletes)
			}
		})
	}
}

func TestDirectoryGroupMappingSurvivingSource(t *testing.T) {
	b := mappingTestBackend(t, true)
	first := mappingConfig("source-a", "shared-target", "5s")
	survivor := strings.Replace(mappingConfig("source-b", "shared-target", "5s"), `"test"`, `"survivor"`, 1)
	survivorResource := survivor[strings.Index(survivor, `resource "rwx_directory_group_mapping"`):]
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
		{Config: first + survivorResource},
		{Config: mappingConfig("source-a", "different-target", "5s") + survivorResource},
		{Config: survivor, Check: resource.TestCheckResourceAttr("rwx_directory_group_mapping.survivor", "group_id", "shared-target")},
		{Config: survivor, PlanOnly: true},
	}})
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.mappings) != 0 {
		t.Fatalf("mappings remain: %v", b.mappings)
	}
}

func TestDirectoryGroupMappingInvalidTimeout(t *testing.T) {
	mappingTestBackend(t, false)
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
		{Config: mappingConfig("source-a", "target-a", "0s"), PlanOnly: true, ExpectError: regexp.MustCompile("must be a positive duration")},
	}})
}
