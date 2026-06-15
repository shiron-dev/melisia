package provider

import (
	"context"
	"os"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/shiron-dev/melisia/terraform-provider-truenas/internal/client"
)

var _ provider.Provider = &TrueNASProvider{}

type TrueNASProvider struct {
	version string
}

type providerModel struct {
	BaseURL               types.String `tfsdk:"base_url"`
	APIKey                types.String `tfsdk:"api_key"`
	TLSInsecureSkipVerify types.Bool   `tfsdk:"tls_insecure_skip_verify"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &TrueNASProvider{version: version}
	}
}

func (p *TrueNASProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "truenas"
	resp.Version = p.version
}

func (p *TrueNASProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"base_url": schema.StringAttribute{
				Optional:    true,
				Description: "Base URL of the TrueNAS SCALE instance. Can also be set with TRUENAS_BASE_URL.",
			},
			"api_key": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "TrueNAS API key. Can also be set with TRUENAS_API_KEY.",
			},
			"tls_insecure_skip_verify": schema.BoolAttribute{
				Optional:    true,
				Description: "Skip TLS certificate verification. Can also be set with TRUENAS_TLS_INSECURE_SKIP_VERIFY.",
			},
		},
	}
}

func (p *TrueNASProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	baseURL := os.Getenv("TRUENAS_BASE_URL")
	if !config.BaseURL.IsNull() {
		baseURL = config.BaseURL.ValueString()
	}

	apiKey := os.Getenv("TRUENAS_API_KEY")
	if !config.APIKey.IsNull() {
		apiKey = config.APIKey.ValueString()
	}

	tlsInsecureSkipVerify := false
	if envValue := os.Getenv("TRUENAS_TLS_INSECURE_SKIP_VERIFY"); envValue != "" {
		parsed, err := strconv.ParseBool(envValue)
		if err != nil {
			resp.Diagnostics.AddError("Invalid TRUENAS_TLS_INSECURE_SKIP_VERIFY value", err.Error())
			return
		}
		tlsInsecureSkipVerify = parsed
	}
	if !config.TLSInsecureSkipVerify.IsNull() {
		tlsInsecureSkipVerify = config.TLSInsecureSkipVerify.ValueBool()
	}

	truenasClient, err := client.New(baseURL, apiKey, tlsInsecureSkipVerify)
	if err != nil {
		resp.Diagnostics.AddError("Unable to configure TrueNAS client", err.Error())
		return
	}

	resp.DataSourceData = truenasClient
	resp.ResourceData = truenasClient
}

func (p *TrueNASProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewAppConfigDocumentDataSource,
		NewPoolDataSource,
	}
}

func (p *TrueNASProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewAppConfigResource,
		NewAppsConfigResource,
		NewDatasetResource,
		NewFilesystemACLResource,
		NewFilesystemPermissionResource,
		NewFilesystemPermissionCopyResource,
		NewSMBShareCopyResource,
	}
}
