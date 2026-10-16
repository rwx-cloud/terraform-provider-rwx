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
	_ datasource.DataSource              = &GroupDataSource{}
	_ datasource.DataSourceWithConfigure = &GroupDataSource{}
)

func NewGroupDataSource() datasource.DataSource {
	return &GroupDataSource{}
}

type GroupDataSource struct {
	client api.Client
}

func (d *GroupDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (d *GroupDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up an RWX group by its stable ID or exact name. Specify exactly one of `id` and `name`. Missing or ambiguous matches fail during planning when the selector is known.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The stable ID of the group. Specify either id or name.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.ExactlyOneOf(path.MatchRoot("name")),
				},
			},
			"name": schema.StringAttribute{
				Description: "The exact, case-sensitive name of the group. Specify either id or name.",
				Optional:    true,
				Computed:    true,
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"description": schema.StringAttribute{
				Description: "The description of the group.",
				Computed:    true,
			},
		},
	}
}

func (d *GroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *GroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config GroupModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var group api.Group
	var err error
	if !config.ID.IsNull() {
		group, err = d.client.GetGroup(config.ID.ValueString())
	} else {
		group, err = d.client.FindGroupByName(config.Name.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Error looking up group in RWX", err.Error())
		return
	}
	state := groupModel(group)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
