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
	_ resource.Resource                = &datasetResource{}
	_ resource.ResourceWithConfigure   = &datasetResource{}
	_ resource.ResourceWithImportState = &datasetResource{}
)

type datasetResource struct {
	client *client.Client
}

type datasetResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Type             types.String `tfsdk:"type"`
	Atime            types.String `tfsdk:"atime"`
	ACLMode          types.String `tfsdk:"aclmode"`
	ACLType          types.String `tfsdk:"acltype"`
	Compression      types.String `tfsdk:"compression"`
	Copies           types.Int64  `tfsdk:"copies"`
	Deduplication    types.String `tfsdk:"deduplication"`
	Exec             types.String `tfsdk:"exec"`
	ForceDestroy     types.Bool   `tfsdk:"force_destroy"`
	Readonly         types.String `tfsdk:"readonly"`
	Recordsize       types.String `tfsdk:"recordsize"`
	Snapdir          types.String `tfsdk:"snapdir"`
	Sync             types.String `tfsdk:"sync"`
	RecursiveDestroy types.Bool   `tfsdk:"recursive_destroy"`
}

func NewDatasetResource() resource.Resource {
	return &datasetResource{}
}

func (r *datasetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dataset"
}

func (r *datasetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a TrueNAS ZFS dataset.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Dataset ID, matching its full name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Full dataset name, such as tank/users.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Required:    true,
				Description: "Dataset type.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"atime": schema.StringAttribute{
				Required:    true,
				Description: "Dataset atime property.",
			},
			"aclmode": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Dataset aclmode property.",
			},
			"acltype": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Dataset acltype property.",
			},
			"compression": schema.StringAttribute{
				Required:    true,
				Description: "Dataset compression property.",
			},
			"copies": schema.Int64Attribute{
				Required:    true,
				Description: "Dataset copies property.",
			},
			"deduplication": schema.StringAttribute{
				Required:    true,
				Description: "Dataset deduplication property.",
			},
			"exec": schema.StringAttribute{
				Required:    true,
				Description: "Dataset exec property.",
			},
			"force_destroy": schema.BoolAttribute{
				Required:    true,
				Description: "Whether Terraform may force dataset deletion.",
			},
			"readonly": schema.StringAttribute{
				Required:    true,
				Description: "Dataset readonly property.",
			},
			"recordsize": schema.StringAttribute{
				Required:    true,
				Description: "Dataset recordsize property.",
			},
			"snapdir": schema.StringAttribute{
				Required:    true,
				Description: "Dataset snapdir property.",
			},
			"sync": schema.StringAttribute{
				Required:    true,
				Description: "Dataset sync property.",
			},
			"recursive_destroy": schema.BoolAttribute{
				Required:    true,
				Description: "Whether Terraform may recursively delete child datasets.",
			},
		},
	}
}

func (r *datasetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *datasetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan datasetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateDataset(ctx, modelToDataset(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to create TrueNAS dataset", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, datasetToModel(created, plan))...)
}

func (r *datasetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state datasetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	dataset, err := r.client.GetDataset(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read TrueNAS dataset", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, datasetToModel(dataset, state))...)
}

func (r *datasetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan datasetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateDataset(ctx, modelToDataset(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to update TrueNAS dataset", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, datasetToModel(updated, plan))...)
}

func (r *datasetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state datasetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteDataset(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete TrueNAS dataset", err.Error())
	}
}

func (r *datasetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func modelToDataset(model datasetResourceModel) client.Dataset {
	id := model.ID.ValueString()
	if id == "" {
		id = model.Name.ValueString()
	}

	return client.Dataset{
		ID:            id,
		Name:          model.Name.ValueString(),
		Type:          model.Type.ValueString(),
		Atime:         model.Atime.ValueString(),
		ACLMode:       model.ACLMode.ValueString(),
		ACLType:       model.ACLType.ValueString(),
		Compression:   model.Compression.ValueString(),
		Copies:        model.Copies.ValueInt64(),
		Deduplication: model.Deduplication.ValueString(),
		Exec:          model.Exec.ValueString(),
		Readonly:      model.Readonly.ValueString(),
		Recordsize:    model.Recordsize.ValueString(),
		Snapdir:       model.Snapdir.ValueString(),
		Sync:          model.Sync.ValueString(),
	}
}

func datasetToModel(dataset client.Dataset, fallback datasetResourceModel) datasetResourceModel {
	copies := dataset.Copies
	if copies == 0 {
		copies = fallback.Copies.ValueInt64()
	}

	return datasetResourceModel{
		ID:               types.StringValue(firstNonEmpty(dataset.ID, dataset.Name, fallback.ID.ValueString(), fallback.Name.ValueString())),
		Name:             types.StringValue(firstNonEmpty(dataset.Name, fallback.Name.ValueString())),
		Type:             types.StringValue(firstNonEmpty(dataset.Type, fallback.Type.ValueString())),
		Atime:            types.StringValue(firstNonEmpty(dataset.Atime, fallback.Atime.ValueString())),
		ACLMode:          types.StringValue(firstNonEmpty(dataset.ACLMode, fallback.ACLMode.ValueString())),
		ACLType:          types.StringValue(firstNonEmpty(dataset.ACLType, fallback.ACLType.ValueString())),
		Compression:      types.StringValue(firstNonEmpty(dataset.Compression, fallback.Compression.ValueString())),
		Copies:           types.Int64Value(copies),
		Deduplication:    types.StringValue(firstNonEmpty(dataset.Deduplication, fallback.Deduplication.ValueString())),
		Exec:             types.StringValue(firstNonEmpty(dataset.Exec, fallback.Exec.ValueString())),
		ForceDestroy:     types.BoolValue(fallback.ForceDestroy.ValueBool()),
		Readonly:         types.StringValue(firstNonEmpty(dataset.Readonly, fallback.Readonly.ValueString())),
		Recordsize:       types.StringValue(firstNonEmpty(dataset.Recordsize, fallback.Recordsize.ValueString())),
		Snapdir:          types.StringValue(firstNonEmpty(dataset.Snapdir, fallback.Snapdir.ValueString())),
		Sync:             types.StringValue(firstNonEmpty(dataset.Sync, fallback.Sync.ValueString())),
		RecursiveDestroy: types.BoolValue(fallback.RecursiveDestroy.ValueBool()),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}
