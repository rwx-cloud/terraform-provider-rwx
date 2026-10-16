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
	_ datasource.DataSource              = &DirectoryGroupDataSource{}
	_ datasource.DataSourceWithConfigure = &DirectoryGroupDataSource{}
)

func NewDirectoryGroupDataSource() datasource.DataSource {
	return &DirectoryGroupDataSource{}
}

type DirectoryGroupDataSource struct {
	client api.Client
}

type DirectoryGroupDataSourceModel struct {
	DirectoryID types.String `tfsdk:"directory_id"`
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
}

func (d *DirectoryGroupDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_directory_group"
}

func (d *DirectoryGroupDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up an existing identity-provider group in an RWX directory. Configure `directory_id` and exactly one of `name` or `id`. This data source does not provision groups or manage IdP-owned rosters. Name lookups must match exactly one group in the directory. Use the stable local RWX `id` to retain identity across renames; ID lookups never fall back to a same-named replacement. Requires `org:access`.",
		Attributes: map[string]schema.Attribute{
			"directory_id": schema.StringAttribute{
				Description: "The local RWX directory ID. The directory must belong to the authenticated organization.",
				Required:    true,
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"id": schema.StringAttribute{
				Description: "The stable local RWX directory-group ID, not the external IdP ID. Specify either id or name.",
				Optional:    true,
				Computed:    true,
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1), stringvalidator.ExactlyOneOf(path.MatchRoot("name"))},
			},
			"name": schema.StringAttribute{
				Description: "The exact, case-sensitive group name within the directory. Specify either id or name. When looking up by ID, returns the current name.",
				Optional:    true,
				Computed:    true,
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
			},
		},
	}
}

func (d *DirectoryGroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *DirectoryGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data DirectoryGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var group api.DirectoryGroup
	var err error
	if !data.ID.IsNull() {
		group, err = d.client.GetDirectoryGroup(data.DirectoryID.ValueString(), data.ID.ValueString())
	} else {
		group, err = d.client.FindDirectoryGroupByName(data.DirectoryID.ValueString(), data.Name.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Error looking up directory group in RWX", err.Error())
		return
	}
	data.ID = types.StringValue(group.ID)
	data.Name = types.StringValue(group.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
