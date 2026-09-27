package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/unmango/terraform-provider-atproto/internal/atproto"
)

var (
	_ datasource.DataSourceWithConfigure = &identityDataSource{}
	_ datasource.DataSourceWithConfigure = &accountDataSource{}
)

// identityDataSource reports the account the provider is logged in as.
type identityDataSource struct {
	client *atproto.Client
}

type identityModel struct {
	DID types.String `tfsdk:"did"`
}

func NewIdentityDataSource() datasource.DataSource {
	return &identityDataSource{}
}

func (d *identityDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_identity"
}

func (d *identityDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The account the provider is logged in as.",
		Attributes: map[string]schema.Attribute{
			"did": schema.StringAttribute{
				MarkdownDescription: "DID of the account.",
				Computed:            true,
			},
		},
	}
}

func (d *identityDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	resp.Diagnostics.Append(configureClient(req.ProviderData, &d.client)...)
}

func (d *identityDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	resp.Diagnostics.Append(resp.State.Set(ctx, identityModel{DID: types.StringValue(d.client.DID())})...)
}

// accountDataSource resolves a handle to a DID.
type accountDataSource struct {
	client *atproto.Client
}

type accountModel struct {
	Handle types.String `tfsdk:"handle"`
	DID    types.String `tfsdk:"did"`
}

func NewAccountDataSource() datasource.DataSource {
	return &accountDataSource{}
}

func (d *accountDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account"
}

func (d *accountDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Resolves a handle to the DID of its account.",
		Attributes: map[string]schema.Attribute{
			"handle": schema.StringAttribute{
				MarkdownDescription: "Handle to resolve, such as `alice.bsky.social`.",
				Required:            true,
			},
			"did": schema.StringAttribute{
				MarkdownDescription: "DID of the account.",
				Computed:            true,
			},
		},
	}
}

func (d *accountDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	resp.Diagnostics.Append(configureClient(req.ProviderData, &d.client)...)
}

func (d *accountDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model accountModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	did, err := d.client.ResolveHandle(ctx, model.Handle.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to Resolve Handle", model.Handle.ValueString()+": "+err.Error())
		return
	}

	model.DID = types.StringValue(did)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
