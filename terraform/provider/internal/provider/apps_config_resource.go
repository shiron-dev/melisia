package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/shiron-dev/melisia/terraform-provider-truenas/internal/client"
)

var (
	_ resource.Resource                = &appsConfigResource{}
	_ resource.ResourceWithConfigure   = &appsConfigResource{}
	_ resource.ResourceWithImportState = &appsConfigResource{}
)

type appsConfigResource struct {
	client *client.Client
}

type appsConfigResourceModel struct {
	ID                 types.String                 `tfsdk:"id"`
	EnableImageUpdates types.Bool                   `tfsdk:"enable_image_updates"`
	Pool               types.String                 `tfsdk:"pool"`
	Nvidia             types.Bool                   `tfsdk:"nvidia"`
	AddressPools       []appsConfigAddressPoolModel `tfsdk:"address_pools"`
	PreferredTrains    []types.String               `tfsdk:"preferred_trains"`
}

type appsConfigAddressPoolModel struct {
	Base types.String `tfsdk:"base"`
	Size types.Int64  `tfsdk:"size"`
}

func NewAppsConfigResource() resource.Resource {
	return &appsConfigResource{}
}

func (r *appsConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_apps_config"
}

func (r *appsConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages global TrueNAS Apps settings. This does not manage installed apps.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Apps config ID. Always apps.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"enable_image_updates": schema.BoolAttribute{
				Required:    true,
				Description: "Whether TrueNAS should check Docker image updates for apps.",
			},
			"pool": schema.StringAttribute{
				Required:    true,
				Description: "Pool used by TrueNAS Apps.",
			},
			"nvidia": schema.BoolAttribute{
				Required:    true,
				Description: "Whether NVIDIA support is enabled for apps.",
			},
			"address_pools": schema.ListNestedAttribute{
				Required:    true,
				Description: "Docker address pools used by TrueNAS Apps.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"base": schema.StringAttribute{
							Required:    true,
							Description: "Base CIDR for the Docker address pool.",
						},
						"size": schema.Int64Attribute{
							Required:    true,
							Description: "Prefix size for networks allocated from the address pool.",
						},
					},
				},
			},
			"preferred_trains": schema.ListAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "Preferred TrueNAS Apps catalog trains.",
			},
		},
	}
}

func (r *appsConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	truenasClient, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected resource configure type", "Expected *client.Client.")
		return
	}

	r.client = truenasClient
}

func (r *appsConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan appsConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateAppsConfig(ctx, modelToAppsConfig(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to update TrueNAS Apps config", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, appsConfigToModel(updated))...)
}

func (r *appsConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	config, err := r.client.GetAppsConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read TrueNAS Apps config", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, appsConfigToModel(config))...)
}

func (r *appsConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan appsConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateAppsConfig(ctx, modelToAppsConfig(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to update TrueNAS Apps config", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, appsConfigToModel(updated))...)
}

func (r *appsConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddError("Unable to delete TrueNAS Apps config", "Global TrueNAS Apps settings cannot be deleted.")
}

func (r *appsConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func modelToAppsConfig(model appsConfigResourceModel) client.AppsConfig {
	return client.AppsConfig{
		ID:                 firstNonEmpty(model.ID.ValueString(), "apps"),
		EnableImageUpdates: model.EnableImageUpdates.ValueBool(),
		Pool:               model.Pool.ValueString(),
		Nvidia:             model.Nvidia.ValueBool(),
		AddressPools:       modelToAppsAddressPools(model.AddressPools),
		PreferredTrains:    modelToPreferredTrains(model.PreferredTrains),
	}
}

func appsConfigToModel(config client.AppsConfig) appsConfigResourceModel {
	return appsConfigResourceModel{
		ID:                 types.StringValue(firstNonEmpty(config.ID, "apps")),
		EnableImageUpdates: types.BoolValue(config.EnableImageUpdates),
		Pool:               types.StringValue(config.Pool),
		Nvidia:             types.BoolValue(config.Nvidia),
		AddressPools:       appsAddressPoolsToModel(config.AddressPools),
		PreferredTrains:    preferredTrainsToModel(config.PreferredTrains),
	}
}

func modelToAppsAddressPools(pools []appsConfigAddressPoolModel) []client.AppsAddressPool {
	result := make([]client.AppsAddressPool, 0, len(pools))
	for _, pool := range pools {
		result = append(result, client.AppsAddressPool{
			Base: pool.Base.ValueString(),
			Size: pool.Size.ValueInt64(),
		})
	}

	return result
}

func appsAddressPoolsToModel(pools []client.AppsAddressPool) []appsConfigAddressPoolModel {
	result := make([]appsConfigAddressPoolModel, 0, len(pools))
	for _, pool := range pools {
		result = append(result, appsConfigAddressPoolModel{
			Base: types.StringValue(pool.Base),
			Size: types.Int64Value(pool.Size),
		})
	}

	return result
}

func modelToPreferredTrains(trains []types.String) []string {
	result := make([]string, 0, len(trains))
	for _, train := range trains {
		result = append(result, train.ValueString())
	}

	return result
}

func preferredTrainsToModel(trains []string) []types.String {
	result := make([]types.String, 0, len(trains))
	for _, train := range trains {
		result = append(result, types.StringValue(train))
	}

	return result
}
