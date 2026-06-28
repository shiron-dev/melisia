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
	_ resource.Resource                = &customAppResource{}
	_ resource.ResourceWithConfigure   = &customAppResource{}
	_ resource.ResourceWithImportState = &customAppResource{}
)

type customAppResource struct {
	client *client.Client
}

type customAppResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	ComposeConfig types.String `tfsdk:"compose_config"`
}

func NewCustomAppResource() resource.Resource {
	return &customAppResource{}
}

func (r *customAppResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_app"
}

func (r *customAppResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a custom (docker-compose) TrueNAS app. Unlike truenas_app_config, which only configures an already-installed catalog app, this resource installs, updates, and removes the custom app itself.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "App ID, matching the app name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "TrueNAS custom app name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"compose_config": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "Canonical JSON of the docker-compose document, sent to TrueNAS as custom_compose_config. Feed this from the truenas_app_config_document data source so the value matches the canonical form TrueNAS returns.",
			},
		},
	}
}

func (r *customAppResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *customAppResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan customAppResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Name.ValueString()
	if err := r.client.CreateCustomApp(ctx, name, json.RawMessage(plan.ComposeConfig.ValueString())); err != nil {
		resp.Diagnostics.AddError("Unable to create TrueNAS custom app", err.Error())
		return
	}

	config, err := r.client.GetAppConfig(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read TrueNAS custom app after create", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, customAppToModel(name, config))...)
}

func (r *customAppResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state customAppResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := firstNonEmpty(state.Name.ValueString(), state.ID.ValueString())
	config, err := r.client.GetAppConfig(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read TrueNAS custom app", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, customAppToModel(name, config))...)
}

func (r *customAppResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan customAppResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Name.ValueString()
	if err := r.client.UpdateCustomApp(ctx, name, json.RawMessage(plan.ComposeConfig.ValueString())); err != nil {
		resp.Diagnostics.AddError("Unable to update TrueNAS custom app", err.Error())
		return
	}

	config, err := r.client.GetAppConfig(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read TrueNAS custom app after update", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, customAppToModel(name, config))...)
}

func (r *customAppResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state customAppResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := firstNonEmpty(state.Name.ValueString(), state.ID.ValueString())
	if err := r.client.DeleteCustomApp(ctx, name); err != nil {
		resp.Diagnostics.AddError("Unable to delete TrueNAS custom app", err.Error())
		return
	}
}

func (r *customAppResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func customAppToModel(name string, config client.AppConfig) customAppResourceModel {
	return customAppResourceModel{
		ID:            types.StringValue(name),
		Name:          types.StringValue(name),
		ComposeConfig: types.StringValue(string(config.Values)),
	}
}
