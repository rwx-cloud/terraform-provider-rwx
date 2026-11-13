package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestVaultChildDeleteIdentity(t *testing.T) {
	for _, typeName := range []string{"rwx_secret", "rwx_variable"} {
		for _, version := range []string{"1.10.4", "1.0.0+pulumi-terraform-bridge"} {
			for _, tc := range []struct {
				name, id, explicitID, privateID string
				wantError                       bool
			}{
				{"canonical without private", "vault_id/original/child", "", "", false},
				{"explicit ID", "", "original", "", false},
				{"private legacy identity", "production/child", "", "original", false},
				{"explicit ID precedence", "vault_id/reused/child", "original", "", false},
				{"private identity precedence", "vault_id/reused/child", "", "original", false},
				{"legacy ambiguous", "production/child", "", "", true},
				{"legacy missing ID", "", "", "", true},
				{"wrong child", "vault_id/original/other", "", "", true},
			} {
				t.Run(typeName+"/"+version+"/"+tc.name, func(t *testing.T) {
					ctx := context.Background()
					children := map[string]bool{"original": true, "reused": true}
					previousClient := http.DefaultClient
					http.DefaultClient = &http.Client{Transport: directoryGroupTransport(func(req *http.Request) (*http.Response, error) {
						body := ""
						if req.Method == http.MethodDelete {
							delete(children, req.URL.Query().Get("vault_id"))
						} else {
							body = `{"vaults":[{"id":"reused","name":"production"}]}`
						}
						return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
					})}
					defer func() { http.DefaultClient = previousClient }()
					protocol := providerserver.NewProtocol6(New("test")())()
					schema, err := protocol.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
					if err != nil {
						t.Fatal(err)
					}
					config, err := tfprotov6.NewDynamicValue(schema.Provider.ValueType(), tftypes.NewValue(schema.Provider.ValueType(), map[string]tftypes.Value{
						"host": tftypes.NewValue(tftypes.String, "vault.test"), "access_token": tftypes.NewValue(tftypes.String, "fixture"),
					}))
					if err != nil {
						t.Fatal(err)
					}
					configured, err := protocol.ConfigureProvider(ctx, &tfprotov6.ConfigureProviderRequest{Config: &config, TerraformVersion: version})
					if err != nil || len(configured.Diagnostics) != 0 {
						t.Fatalf("configure: %v %v", configured, err)
					}
					resourceType := schema.ResourceSchemas[typeName].ValueType()
					attributes := map[string]tftypes.Value{}
					for name, attributeType := range resourceType.(tftypes.Object).AttributeTypes {
						attributes[name] = tftypes.NewValue(attributeType, nil)
					}
					for name, value := range map[string]string{"id": tc.id, "vault": "production", "name": "child"} {
						attributes[name] = tftypes.NewValue(tftypes.String, value)
					}
					if tc.explicitID != "" {
						attributes["vault"] = tftypes.NewValue(tftypes.String, nil)
						attributes["vault_id"] = tftypes.NewValue(tftypes.String, tc.explicitID)
					}
					current, err := tfprotov6.NewDynamicValue(resourceType, tftypes.NewValue(resourceType, attributes))
					if err != nil {
						t.Fatal(err)
					}
					absent, err := tfprotov6.NewDynamicValue(resourceType, tftypes.NewValue(resourceType, nil))
					if err != nil {
						t.Fatal(err)
					}
					var private []byte
					if tc.privateID != "" {
						encoded, err := json.Marshal(tc.privateID)
						if err != nil {
							t.Fatal(err)
						}
						private, err = json.Marshal(map[string][]byte{"vault_id": encoded})
						if err != nil {
							t.Fatal(err)
						}
					}
					deleted, err := protocol.ApplyResourceChange(ctx, &tfprotov6.ApplyResourceChangeRequest{TypeName: typeName, PriorState: &current, PlannedState: &absent, Config: &absent, PlannedPrivate: private})
					if err != nil {
						t.Fatal(err)
					}
					value, err := deleted.NewState.Unmarshal(resourceType)
					if err != nil {
						t.Fatal(err)
					}
					hasError := false
					for _, diagnostic := range deleted.Diagnostics {
						hasError = hasError || diagnostic.Severity == tfprotov6.DiagnosticSeverityError
					}
					if hasError != tc.wantError || value.IsNull() == tc.wantError || !children["reused"] || children["original"] != tc.wantError {
						t.Fatalf("wrong deletion: remaining=%v state=%v diagnostics=%v", children, value, deleted.Diagnostics)
					}
				})
			}
		}
	}
}

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
