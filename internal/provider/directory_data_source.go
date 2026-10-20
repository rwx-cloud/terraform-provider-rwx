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
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = &DirectoryDataSource{}
	_ datasource.DataSourceWithConfigure = &DirectoryDataSource{}
)

func NewDirectoryDataSource() datasource.DataSource {
	return &DirectoryDataSource{}
}

type DirectoryDataSource struct {
	client api.Client
}

type DirectoryDataSourceModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func (d *DirectoryDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_directory"
}

func (d *DirectoryDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up an existing connected directory by exact name or local RWX ID. Connect the directory in the RWX UI first. Directory rosters and mappings are not managed by this data source. Pass its `id` to the [rwx_directory_group data source](https://registry.terraform.io/providers/rwx-cloud/rwx/latest/docs/data-sources/directory_group), then use [rwx_directory_group_mapping](https://registry.terraform.io/providers/rwx-cloud/rwx/latest/docs/resources/directory_group_mapping) to map that directory group to an RWX group.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The stable local RWX directory ID, not the external identity-provider ID. Configure exactly one of `id` or `name`; both attributes return the resolved values.",
				Optional:            true,
				Computed:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1), stringvalidator.ExactlyOneOf(path.MatchRoot("name"))},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The exact, case-sensitive directory name. Names must match a single directory; use `id` when names are ambiguous. Configure exactly one of `id` or `name`; both attributes return the resolved values.",
				Optional:            true,
				Computed:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
		},
	}
}

func (d *DirectoryDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *DirectoryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data DirectoryDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var directory api.Directory
	var err error
	if !data.ID.IsNull() {
		directory, err = d.client.GetDirectory(data.ID.ValueString())
	} else {
		directory, err = d.client.FindDirectoryByName(data.Name.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Error looking up directory in RWX", err.Error())
		return
	}
	data.ID = types.StringValue(directory.ID)
	data.Name = types.StringValue(directory.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
