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
	_ resource.Resource                = &filesystemPermissionResource{}
	_ resource.ResourceWithConfigure   = &filesystemPermissionResource{}
	_ resource.ResourceWithImportState = &filesystemPermissionResource{}
)

type filesystemPermissionResource struct {
	client *client.Client
}

type filesystemPermissionResourceModel struct {
	ID   types.String `tfsdk:"id"`
	Path types.String `tfsdk:"path"`
	Mode types.String `tfsdk:"mode"`
	UID  types.Int64  `tfsdk:"uid"`
	GID  types.Int64  `tfsdk:"gid"`
}

func NewFilesystemPermissionResource() resource.Resource {
	return &filesystemPermissionResource{}
}

func (r *filesystemPermissionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_filesystem_permission"
}

func (r *filesystemPermissionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages POSIX owner, group, and mode for a TrueNAS filesystem path.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Target filesystem path.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"path": schema.StringAttribute{
				Required:    true,
				Description: "Target filesystem path to update.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mode": schema.StringAttribute{
				Required:    true,
				Description: "Filesystem mode, such as 770.",
			},
			"uid": schema.Int64Attribute{
				Required:    true,
				Description: "Owner user ID.",
			},
			"gid": schema.Int64Attribute{
				Required:    true,
				Description: "Owner group ID.",
			},
		},
	}
}

func (r *filesystemPermissionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *filesystemPermissionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan filesystemPermissionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.apply(ctx, plan); err != nil {
		resp.Diagnostics.AddError("Unable to set TrueNAS filesystem permissions", err.Error())
		return
	}

	plan.ID = types.StringValue(plan.Path.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *filesystemPermissionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state filesystemPermissionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	pathValue := filesystemPathFromState(state.Path, state.ID)
	current, err := r.client.GetFilesystemStat(ctx, pathValue)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read TrueNAS filesystem permissions", err.Error())
		return
	}

	state.ID = types.StringValue(pathValue)
	state.Path = types.StringValue(pathValue)
	state.Mode = types.StringValue(current.Mode)
	state.UID = types.Int64Value(current.UID)
	state.GID = types.Int64Value(current.GID)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *filesystemPermissionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan filesystemPermissionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.apply(ctx, plan); err != nil {
		resp.Diagnostics.AddError("Unable to set TrueNAS filesystem permissions", err.Error())
		return
	}

	plan.ID = types.StringValue(plan.Path.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *filesystemPermissionResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *filesystemPermissionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *filesystemPermissionResource) apply(ctx context.Context, model filesystemPermissionResourceModel) error {
	return r.client.SetFilesystemPermission(ctx, client.FilesystemStat{
		Path: model.Path.ValueString(),
		Mode: model.Mode.ValueString(),
		UID:  model.UID.ValueInt64(),
		GID:  model.GID.ValueInt64(),
	})
}
