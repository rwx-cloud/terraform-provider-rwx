package provider

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestVaultSelectorsRequireReplace(t *testing.T) {
	tests := []struct {
		name       string
		state      vaultSelectorModel
		plan       vaultSelectorModel
		previousID string
		vaultsJSON string
		want       bool
	}{
		{
			name:  "unchanged name",
			state: vaultSelectorModel{vault: types.StringValue("production"), vaultID: types.StringNull()},
			plan:  vaultSelectorModel{vault: types.StringValue("production"), vaultID: types.StringNull()},
			want:  false,
		},
		{
			name:       "name to matching ID",
			state:      vaultSelectorModel{vault: types.StringValue("production"), vaultID: types.StringNull()},
			plan:       vaultSelectorModel{vault: types.StringNull(), vaultID: types.StringValue("vault-id")},
			previousID: "vault-id",
			want:       false,
		},
		{
			name:       "name to different ID",
			state:      vaultSelectorModel{vault: types.StringValue("production"), vaultID: types.StringNull()},
			plan:       vaultSelectorModel{vault: types.StringNull(), vaultID: types.StringValue("other-vault-id")},
			previousID: "vault-id",
			want:       true,
		},
		{
			name:       "managed vault rename",
			state:      vaultSelectorModel{vault: types.StringValue("old-name"), vaultID: types.StringNull()},
			plan:       vaultSelectorModel{vault: types.StringValue("new-name"), vaultID: types.StringNull()},
			previousID: "vault-id",
			vaultsJSON: `{"vaults":[{"id":"vault-id","name":"new-name"}]}`,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := api.Client{RoundTrip: func(_ *http.Request) (*http.Response, error) {
				if tt.vaultsJSON == "" {
					t.Fatal("unexpected API request")
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     http.StatusText(http.StatusOK),
					Body:       io.NopCloser(strings.NewReader(tt.vaultsJSON)),
				}, nil
			}}
			got, diags := vaultSelectorsRequireReplace(client, tt.state, tt.plan, tt.previousID)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if got != tt.want {
				t.Fatalf("requires replace = %t, want %t", got, tt.want)
			}
		})
	}
}
