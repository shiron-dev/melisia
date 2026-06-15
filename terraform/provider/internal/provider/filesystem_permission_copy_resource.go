package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/shiron-dev/melisia/terraform-provider-truenas/internal/client"
)

var (
	_ resource.Resource              = &filesystemPermissionCopyResource{}
	_ resource.ResourceWithConfigure = &filesystemPermissionCopyResource{}
)

type filesystemPermissionCopyResource struct {
	client *client.Client
}

type filesystemPermissionCopyResourceModel struct {
	ID         types.String `tfsdk:"id"`
	SourcePath types.String `tfsdk:"source_path"`
	Path       types.String `tfsdk:"path"`
	Mode       types.String `tfsdk:"mode"`
	UID        types.Int64  `tfsdk:"uid"`
	GID        types.Int64  `tfsdk:"gid"`
}

func NewFilesystemPermissionCopyResource() resource.Resource {
	return &filesystemPermissionCopyResource{}
}

func (r *filesystemPermissionCopyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_filesystem_permission_copy"
}

func (r *filesystemPermissionCopyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Copies POSIX owner, group, and mode from one TrueNAS filesystem path to another.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Target filesystem path.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"source_path": schema.StringAttribute{
				Required:    true,
				Description: "Source filesystem path to read owner, group, and mode from.",
			},
			"path": schema.StringAttribute{
				Required:    true,
				Description: "Target filesystem path to update.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mode": schema.StringAttribute{
				Computed:    true,
				Description: "Copied filesystem mode.",
			},
			"uid": schema.Int64Attribute{
				Computed:    true,
				Description: "Copied owner user ID.",
			},
			"gid": schema.Int64Attribute{
				Computed:    true,
				Description: "Copied owner group ID.",
			},
		},
	}
}

func (r *filesystemPermissionCopyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *filesystemPermissionCopyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan filesystemPermissionCopyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state, err := r.copyPermission(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Unable to copy TrueNAS filesystem permissions", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *filesystemPermissionCopyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state filesystemPermissionCopyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	current, err := r.client.GetFilesystemStat(ctx, state.Path.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read TrueNAS filesystem permissions", err.Error())
		return
	}

	state.ID = types.StringValue(state.Path.ValueString())
	state.Mode = types.StringValue(current.Mode)
	state.UID = types.Int64Value(current.UID)
	state.GID = types.Int64Value(current.GID)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *filesystemPermissionCopyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan filesystemPermissionCopyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state, err := r.copyPermission(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Unable to copy TrueNAS filesystem permissions", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *filesystemPermissionCopyResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *filesystemPermissionCopyResource) copyPermission(ctx context.Context, plan filesystemPermissionCopyResourceModel) (filesystemPermissionCopyResourceModel, error) {
	source, err := r.client.GetFilesystemStat(ctx, plan.SourcePath.ValueString())
	if err != nil {
		return filesystemPermissionCopyResourceModel{}, err
	}

	target := client.FilesystemStat{
		Path: plan.Path.ValueString(),
		Mode: source.Mode,
		UID:  source.UID,
		GID:  source.GID,
	}
	if err := r.client.SetFilesystemPermission(ctx, target); err != nil {
		return filesystemPermissionCopyResourceModel{}, err
	}

	return filesystemPermissionCopyResourceModel{
		ID:         types.StringValue(target.Path),
		SourcePath: plan.SourcePath,
		Path:       plan.Path,
		Mode:       types.StringValue(target.Mode),
		UID:        types.Int64Value(target.UID),
		GID:        types.Int64Value(target.GID),
	}, nil
}
