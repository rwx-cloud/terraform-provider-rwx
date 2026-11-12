package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"
)

const (
	// Shared config for acceptance tests.
	// We keep this empty and utilize environment variables instead for testing
	//
	// 	RWX_HOST        for setting the host
	//	RWX_ACCESS_TOKEN for the access token
	//
	providerConfig = `
provider "rwx" {}
`
)

var (
	// testAccProtoV6ProviderFactories are used to instantiate a provider during
	// acceptance testing. The factory function will be invoked for every Terraform
	// CLI command executed to create a provider server to which the CLI can
	// reattach.
	testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
		"rwx": providerserver.NewProtocol6WithError(New("test")()),
	}
)

func TestProviderCallerIdentity(t *testing.T) {
	for _, tc := range []struct {
		version string
		pulumi  bool
	}{
		{"1.0.0+pulumi-terraform-bridge", true},
		{"1.5.0+pulumi-terraform-bridge", true},
		{"1.5.0+build.pulumi-terraform-bridge.extra", true},
		{"pulumi-terraform-bridge", true},
		{"1.10.4", false},
		{"1.5.0+other", false},
		{"", false},
	} {
		t.Run(tc.version, func(t *testing.T) {
			ctx := context.Background()
			p := New("test")()
			var schema provider.SchemaResponse
			p.Schema(ctx, provider.SchemaRequest{}, &schema)
			config := tfsdk.Config{Schema: schema.Schema, Raw: tftypes.NewValue(schema.Schema.Type().TerraformType(ctx), map[string]tftypes.Value{
				"host": tftypes.NewValue(tftypes.String, "caller.test"), "access_token": tftypes.NewValue(tftypes.String, "fixture"),
			})}
			var configured provider.ConfigureResponse
			p.Configure(ctx, provider.ConfigureRequest{Config: config, TerraformVersion: tc.version}, &configured)
			if configured.Diagnostics.HasError() {
				t.Fatal(configured.Diagnostics)
			}
			previousClient := http.DefaultClient
			http.DefaultClient = &http.Client{Transport: directoryGroupTransport(func(req *http.Request) (*http.Response, error) {
				wantHeader, otherHeader, wantAgent := api.TerraformManagementHeader, "X-RWX-Pulumi-Management", "terraform-provider-rwx/test"
				if tc.pulumi {
					wantHeader, otherHeader, wantAgent = otherHeader, wantHeader, "pulumi-rwx/test"
				}
				if req.Header.Get(wantHeader) != "claim" || req.Header.Get(otherHeader) != "" || req.UserAgent() != wantAgent {
					t.Fatalf("caller %q sent wrong identity: %v", tc.version, req.Header)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}"))}, nil
			})}
			defer func() { http.DefaultClient = previousClient }()
			client := configured.ResourceData.(api.Client)
			req, err := http.NewRequest(http.MethodGet, "https://caller.test/mint/api/groups", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := client.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
		})
	}
}
