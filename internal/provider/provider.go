package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/unmango/terraform-provider-atproto/internal/atproto"
)

var _ provider.Provider = &atprotoProvider{}

type atprotoProvider struct {
	version string
}

type atprotoProviderModel struct {
	Handle      types.String `tfsdk:"handle"`
	AppPassword types.String `tfsdk:"app_password"`
	PDSHost     types.String `tfsdk:"pds_host"`
}

// login creates the authenticated client. It is a variable so tests can
// substitute one pointed at a fake PDS.
var login = atproto.Login

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &atprotoProvider{version: version}
	}
}

func (p *atprotoProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "atproto"
	resp.Version = p.version
}

func (p *atprotoProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage AT Protocol records, including Tangled knots, repositories, and profiles.\n\n" +
			"Every record is written to the repository of the account the provider logs in as.",
		Attributes: map[string]schema.Attribute{
			"handle": schema.StringAttribute{
				MarkdownDescription: "Handle or DID of the account. May also be set with the `ATPROTO_HANDLE` environment variable.",
				Optional:            true,
			},
			"app_password": schema.StringAttribute{
				MarkdownDescription: "App password for the account. May also be set with the `ATPROTO_APP_PASSWORD` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"pds_host": schema.StringAttribute{
				MarkdownDescription: "URL of the account's PDS, such as `https://bsky.social`. " +
					"Defaults to the PDS in the account's DID document. May also be set with the `ATPROTO_PDS_HOST` environment variable.",
				Optional: true,
			},
		},
	}
}

func (p *atprotoProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config atprotoProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for name, value := range map[string]types.String{
		"handle":       config.Handle,
		"app_password": config.AppPassword,
		"pds_host":     config.PDSHost,
	} {
		if value.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root(name),
				"Unknown Provider Configuration Value",
				"The provider cannot log in because "+name+" is not known at plan time. "+
					"Set it to a static value or apply the resource that produces it first.",
			)
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	cfg := atproto.Config{
		Identifier:  stringOrEnv(config.Handle, "ATPROTO_HANDLE"),
		AppPassword: stringOrEnv(config.AppPassword, "ATPROTO_APP_PASSWORD"),
		PDSHost:     stringOrEnv(config.PDSHost, "ATPROTO_PDS_HOST"),
	}

	if cfg.Identifier == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("handle"),
			"Missing Handle",
			"Set the handle attribute or the ATPROTO_HANDLE environment variable.",
		)
	}
	if cfg.AppPassword == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("app_password"),
			"Missing App Password",
			"Set the app_password attribute or the ATPROTO_APP_PASSWORD environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	client, err := login(ctx, cfg)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Log In", "Logging in as "+cfg.Identifier+" failed: "+err.Error())
		return
	}

	resp.ResourceData = client
	resp.DataSourceData = client
}

func (p *atprotoProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewRecordResource,
		NewTangledFollowResource,
		NewTangledKnotResource,
		NewTangledKnotMemberResource,
		NewTangledProfileResource,
		NewTangledPublicKeyResource,
		NewTangledRepositoryResource,
		NewTangledRepositoryCollaboratorResource,
	}
}

func (p *atprotoProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewIdentityDataSource,
		NewAccountDataSource,
	}
}

func stringOrEnv(value types.String, env string) string {
	if !value.IsNull() {
		return value.ValueString()
	}

	return os.Getenv(env)
}
