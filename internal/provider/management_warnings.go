package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"
)

type managementWarnings struct {
	mu      sync.Mutex
	details map[string]bool
}

func (w *managementWarnings) withWarnings(client api.Client) api.Client {
	roundTrip := client.RoundTrip
	client.RoundTrip = func(req *http.Request) (*http.Response, error) {
		resp, err := roundTrip(req)
		if err != nil {
			return resp, err
		}
		if header := resp.Header.Get("X-RWX-Iac-Warning"); strings.TrimSpace(header) != "" {
			const unreadableWarning = "RWX returned a warning that this provider could not decode."
			var details []string
			var messages []*string
			if json.Unmarshal([]byte(header), &messages) == nil && messages != nil {
				for _, message := range messages {
					if message == nil {
						details = []string{unreadableWarning}
						break
					}
					details = append(details, *message)
				}
			} else {
				var owner struct {
					Source  string   `json:"source"`
					Sources []string `json:"sources"`
				}
				if json.Unmarshal([]byte(header), &owner) == nil {
					for _, source := range append(owner.Sources, owner.Source) {
						if source == "terraform" || source == "pulumi" {
							details = append(details, fmt.Sprintf("RWX attributes this resource to %s. Authorized operations still proceed; changes made by another tool may overwrite this resource.", source))
						}
					}
				}
				if len(details) == 0 {
					details = append(details, unreadableWarning)
				}
			}
			w.mu.Lock()
			if w.details == nil {
				w.details = map[string]bool{}
			}
			for _, detail := range details {
				if strings.TrimSpace(detail) != "" {
					w.details[detail] = true
				}
			}
			w.mu.Unlock()
		}
		return resp, nil
	}
	return client
}

func (w *managementWarnings) reportWarnings(diagnostics *diag.Diagnostics) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for detail := range w.details {
		diagnostics.AddWarning("RWX warning", detail)
	}
	w.details = nil
}
