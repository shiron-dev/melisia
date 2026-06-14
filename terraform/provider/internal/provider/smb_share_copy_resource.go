package provider

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/shiron-dev/melisia/terraform-provider-truenas/internal/client"
)

var (
	_ resource.Resource                = &smbShareCopyResource{}
	_ resource.ResourceWithConfigure   = &smbShareCopyResource{}
	_ resource.ResourceWithImportState = &smbShareCopyResource{}
)

type smbShareCopyResource struct {
	client *client.Client
}

type smbShareCopyResourceModel struct {
	ID         types.String `tfsdk:"id"`
	SourcePath types.String `tfsdk:"source_path"`
	Name       types.String `tfsdk:"name"`
	Path       types.String `tfsdk:"path"`
	Purpose    types.String `tfsdk:"purpose"`
	Enabled    types.Bool   `tfsdk:"enabled"`
	Comment    types.String `tfsdk:"comment"`
}

func NewSMBShareCopyResource() resource.Resource {
	return &smbShareCopyResource{}
}

func (r *smbShareCopyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_smb_share_copy"
}

func (r *smbShareCopyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Creates a TrueNAS SMB share by copying options from an existing share path.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "SMB share numeric ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"source_path": schema.StringAttribute{
				Required:    true,
				Description: "Existing SMB share path to copy options from.",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Target SMB share name.",
			},
			"path": schema.StringAttribute{
				Required:    true,
				Description: "Target SMB share path.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"purpose": schema.StringAttribute{
				Computed:    true,
				Description: "Copied SMB share purpose.",
			},
			"enabled": schema.BoolAttribute{
				Computed:    true,
				Description: "Copied SMB share enabled state.",
			},
			"comment": schema.StringAttribute{
				Computed:    true,
				Description: "Copied SMB share comment.",
			},
		},
	}
}

func (r *smbShareCopyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *smbShareCopyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan smbShareCopyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state, err := r.createOrUpdate(ctx, plan, 0)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create TrueNAS SMB share", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *smbShareCopyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state smbShareCopyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	share, err := r.client.GetSMBShareByPath(ctx, state.Path.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read TrueNAS SMB share", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, smbShareCopyToModel(share, state))...)
}

func (r *smbShareCopyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan smbShareCopyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, _ := strconv.ParseInt(plan.ID.ValueString(), 10, 64)
	state, err := r.createOrUpdate(ctx, plan, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update TrueNAS SMB share", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *smbShareCopyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state smbShareCopyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Unable to parse TrueNAS SMB share ID", err.Error())
		return
	}

	if err := r.client.DeleteSMBShare(ctx, id); err != nil {
		resp.Diagnostics.AddError("Unable to delete TrueNAS SMB share", err.Error())
	}
}

func (r *smbShareCopyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *smbShareCopyResource) createOrUpdate(ctx context.Context, plan smbShareCopyResourceModel, id int64) (smbShareCopyResourceModel, error) {
	source, err := r.client.GetSMBShareByPath(ctx, plan.SourcePath.ValueString())
	if err != nil {
		return smbShareCopyResourceModel{}, err
	}

	target := client.SMBShare{
		ID:      id,
		Name:    plan.Name.ValueString(),
		Path:    plan.Path.ValueString(),
		Purpose: source.Purpose,
		Enabled: source.Enabled,
		Comment: source.Comment,
	}

	var share client.SMBShare
	if id == 0 {
		share, err = r.client.CreateSMBShare(ctx, target)
	} else {
		share, err = r.client.UpdateSMBShare(ctx, target)
	}
	if err != nil {
		return smbShareCopyResourceModel{}, err
	}

	return smbShareCopyToModel(share, plan), nil
}

func smbShareCopyToModel(share client.SMBShare, fallback smbShareCopyResourceModel) smbShareCopyResourceModel {
	return smbShareCopyResourceModel{
		ID:         types.StringValue(strconv.FormatInt(share.ID, 10)),
		SourcePath: fallback.SourcePath,
		Name:       types.StringValue(firstNonEmpty(share.Name, fallback.Name.ValueString())),
		Path:       types.StringValue(firstNonEmpty(share.Path, fallback.Path.ValueString())),
		Purpose:    types.StringValue(firstNonEmpty(share.Purpose, fallback.Purpose.ValueString())),
		Enabled:    types.BoolValue(share.Enabled),
		Comment:    types.StringValue(firstNonEmpty(share.Comment, fallback.Comment.ValueString())),
	}
}
