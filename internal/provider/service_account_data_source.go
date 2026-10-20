package provider

import (
	"context"
	"fmt"

	"github.com/rwx-cloud/terraform-provider-rwx/internal/api"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var (
	_ datasource.DataSource              = &ServiceAccountDataSource{}
	_ datasource.DataSourceWithConfigure = &ServiceAccountDataSource{}
)

func NewServiceAccountDataSource() datasource.DataSource {
	return &ServiceAccountDataSource{}
}

type ServiceAccountDataSource struct {
	client api.Client
}

func (d *ServiceAccountDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_account"
}

func (d *ServiceAccountDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up one active RWX service account by immutable key or ID. Configure exactly one lookup attribute; the remaining attributes return the resolved values. Key matches are exact and case-sensitive. Missing matches fail during planning when inputs are known. Display names cannot be used for lookup because they can change. Does not expose credentials or permissions.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The stable service-account ID. Configure exactly one of `id` or `key`.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.ExactlyOneOf(path.MatchRoot("key")),
				},
			},
			"key": schema.StringAttribute{
				MarkdownDescription: "The immutable service-account key shown in RWX, also used in vault resources and run definitions. Configure exactly one of `id` or `key`.",
				Optional:            true,
				Computed:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "The mutable display name. Read-only; use `id` or `key` for lookup.",
				Computed:            true,
			},
			"identity_id": schema.StringAttribute{
				Description: "The stable identity ID.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "The service-account description.",
				Computed:    true,
			},
		},
	}
}

func (d *ServiceAccountDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(api.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected api.Client, got: %T. Please report this issue to support@rwx.com.", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *ServiceAccountDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ServiceAccountModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var account api.ServiceAccount
	var err error
	if !config.ID.IsNull() {
		account, err = d.client.GetServiceAccount(config.ID.ValueString())
	} else {
		account, err = d.client.FindServiceAccount(config.Key.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Error looking up service account in RWX", err.Error())
		return
	}
	state := serviceAccountModel(account)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
