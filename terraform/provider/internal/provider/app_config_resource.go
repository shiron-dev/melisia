package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/shiron-dev/melisia/terraform-provider-truenas/internal/client"
)

var (
	_ resource.Resource                = &appConfigResource{}
	_ resource.ResourceWithConfigure   = &appConfigResource{}
	_ resource.ResourceWithImportState = &appConfigResource{}
)

type appConfigResource struct {
	client *client.Client
}

type appConfigResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	ConfigJSON types.String `tfsdk:"config_json"`
}

func NewAppConfigResource() resource.Resource {
	return &appConfigResource{}
}

func (r *appConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app_config"
}

func (r *appConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages user-specified configuration values for an installed TrueNAS app.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "App config ID, matching the app name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Installed TrueNAS app name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"config_json": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "Canonical JSON app values sent to TrueNAS app update.",
			},
		},
	}
}

func (r *appConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *appConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan appConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateAppConfig(ctx, modelToAppConfig(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to update TrueNAS app config", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, appConfigToModel(updated))...)
}

func (r *appConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state appConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := firstNonEmpty(state.Name.ValueString(), state.ID.ValueString())
	config, err := r.client.GetAppConfig(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read TrueNAS app config", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, appConfigToModel(config))...)
}

func (r *appConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan appConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateAppConfig(ctx, modelToAppConfig(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to update TrueNAS app config", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, appConfigToModel(updated))...)
}

func (r *appConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddError("Unable to delete TrueNAS app config", "Installed app configuration cannot be deleted independently of the app.")
}

func (r *appConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func modelToAppConfig(model appConfigResourceModel) client.AppConfig {
	return client.AppConfig{
		ID:     firstNonEmpty(model.ID.ValueString(), model.Name.ValueString()),
		Name:   model.Name.ValueString(),
		Values: json.RawMessage(model.ConfigJSON.ValueString()),
	}
}

func appConfigToModel(config client.AppConfig) appConfigResourceModel {
	return appConfigResourceModel{
		ID:         types.StringValue(firstNonEmpty(config.ID, config.Name)),
		Name:       types.StringValue(config.Name),
		ConfigJSON: types.StringValue(string(config.Values)),
	}
}
