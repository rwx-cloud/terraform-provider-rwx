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
	_ datasource.DataSource              = &UserDataSource{}
	_ datasource.DataSourceWithConfigure = &UserDataSource{}
)

func NewUserDataSource() datasource.DataSource {
	return &UserDataSource{}
}

type UserDataSource struct {
	client api.Client
}

type UserDataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	IdentityID types.String `tfsdk:"identity_id"`
	Email      types.String `tfsdk:"email"`
}

func (d *UserDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *UserDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up an existing active user in the authenticated organization by email or stable RWX user ID. Requires `org:access` with a personal or service-account access token. Known selectors resolve during planning; missing or unauthorized lookups fail the plan. This data source does not create or invite users or change memberships.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The stable RWX user ID, not the identity ID. Configure exactly one of `id` or `email`.",
				Optional:            true,
				Computed:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1), stringvalidator.ExactlyOneOf(path.MatchRoot("email"))},
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "The user's email. Lookup uses case-insensitive exact equality, without trimming, aliases, or fuzzy matching. Configure exactly one of `id` or `email`. Configured email casing is preserved; ID lookups return the stored email.",
				Optional:            true,
				Computed:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"identity_id": schema.StringAttribute{
				MarkdownDescription: "The user's RWX identity ID. This is distinct from the stable user `id`.",
				Computed:            true,
			},
		},
	}
}

func (d *UserDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(api.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", fmt.Sprintf("Expected api.Client, got: %T. Please report this issue to support@rwx.com.", req.ProviderData))
		return
	}
	d.client = client
}

func (d *UserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data UserDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var user api.User
	var err error
	if !data.ID.IsNull() {
		user, err = d.client.GetUser(data.ID.ValueString())
	} else {
		user, err = d.client.FindUserByEmail(data.Email.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Error looking up user in RWX", err.Error())
		return
	}
	data.ID = types.StringValue(user.ID)
	data.IdentityID = types.StringValue(user.IdentityID)
	if data.Email.IsNull() {
		data.Email = types.StringValue(user.Email)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
