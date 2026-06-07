package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/shiron-dev/melisia/terraform-provider-truenas/internal/client"
)

var (
	_ datasource.DataSource              = &poolDataSource{}
	_ datasource.DataSourceWithConfigure = &poolDataSource{}
)

type poolDataSource struct {
	client *client.Client
}

type poolDataSourceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Path      types.String `tfsdk:"path"`
	Status    types.String `tfsdk:"status"`
	Healthy   types.Bool   `tfsdk:"healthy"`
	Size      types.Int64  `tfsdk:"size"`
	Available types.Int64  `tfsdk:"available"`
}

func NewPoolDataSource() datasource.DataSource {
	return &poolDataSource{}
}

func (d *poolDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pool"
}

func (d *poolDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a TrueNAS storage pool.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:    true,
				Description: "Pool name.",
			},
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "Pool name.",
			},
			"path": schema.StringAttribute{
				Computed:    true,
				Description: "Pool mount path.",
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Pool status.",
			},
			"healthy": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether the pool is healthy.",
			},
			"size": schema.Int64Attribute{
				Computed:    true,
				Description: "Pool size in bytes.",
			},
			"available": schema.Int64Attribute{
				Computed:    true,
				Description: "Available pool capacity in bytes.",
			},
		},
	}
}

func (d *poolDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	truenasClient, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected data source configure type", "Expected *client.Client.")
		return
	}

	d.client = truenasClient
}

func (d *poolDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config poolDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	pool, err := d.client.GetPool(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read TrueNAS pool", err.Error())
		return
	}

	state := poolDataSourceModel{
		ID:        config.ID,
		Name:      types.StringValue(pool.Name),
		Path:      types.StringValue(pool.Path),
		Status:    types.StringValue(pool.Status),
		Healthy:   types.BoolValue(pool.Healthy),
		Size:      types.Int64Value(pool.Size),
		Available: types.Int64Value(pool.Available),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
