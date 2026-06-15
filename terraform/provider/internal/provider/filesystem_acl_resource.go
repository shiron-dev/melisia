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
	_ resource.Resource                = &filesystemACLResource{}
	_ resource.ResourceWithConfigure   = &filesystemACLResource{}
	_ resource.ResourceWithImportState = &filesystemACLResource{}
)

type filesystemACLResource struct {
	client *client.Client
}

type filesystemACLResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Path      types.String `tfsdk:"path"`
	UID       types.Int64  `tfsdk:"uid"`
	GID       types.Int64  `tfsdk:"gid"`
	ACLType   types.String `tfsdk:"acltype"`
	ACLJSON   types.String `tfsdk:"acl_json"`
	Recursive types.Bool   `tfsdk:"recursive"`
}

func NewFilesystemACLResource() resource.Resource {
	return &filesystemACLResource{}
}

func (r *filesystemACLResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_filesystem_acl"
}

func (r *filesystemACLResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages owner, group, ACL type, and ACL entries for a TrueNAS filesystem path.",
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
			"uid": schema.Int64Attribute{
				Required:    true,
				Description: "Owner user ID.",
			},
			"gid": schema.Int64Attribute{
				Required:    true,
				Description: "Owner group ID.",
			},
			"acltype": schema.StringAttribute{
				Required:    true,
				Description: "Filesystem ACL type.",
			},
			"acl_json": schema.StringAttribute{
				Required:    true,
				Description: "JSON encoded ACL entries.",
			},
			"recursive": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether to apply ACL changes recursively.",
			},
		},
	}
}

func (r *filesystemACLResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *filesystemACLResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan filesystemACLResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.apply(ctx, plan); err != nil {
		resp.Diagnostics.AddError("Unable to set TrueNAS filesystem ACL", err.Error())
		return
	}

	plan.ID = types.StringValue(plan.Path.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *filesystemACLResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state filesystemACLResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	pathValue := filesystemPathFromState(state.Path, state.ID)
	current, err := r.client.GetFilesystemACL(ctx, pathValue)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read TrueNAS filesystem ACL", err.Error())
		return
	}

	state.ID = types.StringValue(pathValue)
	state.Path = types.StringValue(pathValue)
	state.UID = types.Int64Value(current.UID)
	state.GID = types.Int64Value(current.GID)
	state.ACLType = types.StringValue(current.ACLType)
	state.ACLJSON = types.StringValue(string(current.ACL))
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *filesystemACLResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan filesystemACLResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.apply(ctx, plan); err != nil {
		resp.Diagnostics.AddError("Unable to set TrueNAS filesystem ACL", err.Error())
		return
	}

	plan.ID = types.StringValue(plan.Path.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *filesystemACLResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *filesystemACLResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *filesystemACLResource) apply(ctx context.Context, model filesystemACLResourceModel) error {
	return r.client.SetFilesystemACL(ctx, client.FilesystemACL{
		Path:      model.Path.ValueString(),
		UID:       model.UID.ValueInt64(),
		GID:       model.GID.ValueInt64(),
		ACLType:   model.ACLType.ValueString(),
		ACL:       json.RawMessage(model.ACLJSON.ValueString()),
		Recursive: model.Recursive.ValueBool(),
	})
}

func filesystemPathFromState(pathValue, idValue types.String) string {
	if !pathValue.IsNull() && !pathValue.IsUnknown() && pathValue.ValueString() != "" {
		return pathValue.ValueString()
	}
	if !idValue.IsNull() && !idValue.IsUnknown() {
		return idValue.ValueString()
	}
	return ""
}
