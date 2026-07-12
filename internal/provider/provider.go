package provider

import (
	"context"
	"os"

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure RwxProvider satisfies the provider interface.
var _ provider.Provider = &RwxProvider{}

type RwxProvider struct {
	version string
}

// RwxProviderModel describes the provider data model.
type RwxProviderModel struct {
	Host        types.String `tfsdk:"host"`
	AccessToken types.String `tfsdk:"access_token"`
}

func (p *RwxProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "rwx"
	resp.Version = p.version
}

func (p *RwxProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The RWX provider enables Terraform to manage RWX resources such as vault secrets and variables.",
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				Description: "The URI for RWX's API. Default: cloud.rwx.com. This attribute may also be provided via the RWX_HOST environment variable. It is usually only needed for testing or development of the Terraform provider itself.",
				Optional:    true,
			},
			"access_token": schema.StringAttribute{
				Description: "The access token for RWX's API. This may also be provided via the RWX_ACCESS_TOKEN environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
		},
	}
}

func (p *RwxProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config RwxProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.Host.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("host"),
			"Unknown RWX Host",
			"The provider cannot create the RWX API client as there is an unknown configuration value for the RWX host. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the RWX_HOST environment variable.",
		)
	}
	if config.AccessToken.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("access_token"),
			"Unknown RWX Access Token",
			"The provider cannot create the RWX API client as there is an unknown configuration value for the RWX access token. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the RWX_ACCESS_TOKEN environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	host := os.Getenv("RWX_HOST")
	accessToken := os.Getenv("RWX_ACCESS_TOKEN")

	if !config.Host.IsNull() {
		host = config.Host.ValueString()
	}
	if !config.AccessToken.IsNull() {
		accessToken = config.AccessToken.ValueString()
	}

	if host == "" {
		host = "cloud.rwx.com"
	}
	if accessToken == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("access_token"),
			"Missing RWX Access Token",
			"The provider cannot create the RWX API client as there is a missing or empty value for the RWX access token. "+
				"Set the access token value in the configuration or use the RWX_ACCESS_TOKEN environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	client, err := api.NewClient(api.Config{Host: host, AccessToken: accessToken, Version: p.version})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create RWX API client",
			"An unexpected error occurred when creating the RWX API client. "+
				"If the error is not clear, please contact us at support@rwx.com.\n\n"+
				"Original Error: "+err.Error(),
		)
	}

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *RwxProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewSecretResource,
		NewVariableResource,
	}
}

func (p *RwxProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return nil
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &RwxProvider{
			version: version,
		}
	}
}
