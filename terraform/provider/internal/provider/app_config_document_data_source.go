package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var _ datasource.DataSource = &appConfigDocumentDataSource{}

type appConfigDocumentDataSource struct{}

type appConfigDocumentDataSourceModel struct {
	Config types.Dynamic `tfsdk:"config"`
	JSON   types.String  `tfsdk:"json"`
}

func NewAppConfigDocumentDataSource() datasource.DataSource {
	return &appConfigDocumentDataSource{}
}

func (d *appConfigDocumentDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app_config_document"
}

func (d *appConfigDocumentDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Builds canonical JSON for a TrueNAS app configuration from a Terraform object.",
		Attributes: map[string]schema.Attribute{
			"config": schema.DynamicAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "Terraform object describing the TrueNAS app configuration.",
			},
			"json": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "Canonical JSON representation of the app configuration.",
			},
		},
	}
}

func (d *appConfigDocumentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model appConfigDocumentDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config, err := dynamicToJSONValue(ctx, model.Config)
	if err != nil {
		resp.Diagnostics.AddError("Unable to encode TrueNAS app config document", err.Error())
		return
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		resp.Diagnostics.AddError("Unable to encode TrueNAS app config document", err.Error())
		return
	}

	model.JSON = types.StringValue(string(configJSON))
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func dynamicToJSONValue(ctx context.Context, value types.Dynamic) (any, error) {
	if value.IsUnknown() || value.IsUnderlyingValueUnknown() {
		return nil, fmt.Errorf("config contains unknown values")
	}
	if value.IsNull() || value.IsUnderlyingValueNull() {
		return nil, nil
	}

	underlying := value.UnderlyingValue()
	if underlying == nil {
		return nil, fmt.Errorf("config has no underlying value")
	}

	terraformValue, err := underlying.ToTerraformValue(ctx)
	if err != nil {
		return nil, err
	}

	return terraformValueToJSONValue(terraformValue)
}

func terraformValueToJSONValue(value tftypes.Value) (any, error) {
	if !value.IsKnown() {
		return nil, fmt.Errorf("config contains unknown values")
	}
	if value.IsNull() {
		return nil, nil
	}

	valueType := value.Type()
	switch {
	case valueType.Is(tftypes.String):
		var result string
		if err := value.As(&result); err != nil {
			return nil, err
		}
		return result, nil
	case valueType.Is(tftypes.Number):
		result := big.NewFloat(0)
		if err := value.As(&result); err != nil {
			return nil, err
		}
		return json.Number(result.Text('f', -1)), nil
	case valueType.Is(tftypes.Bool):
		var result bool
		if err := value.As(&result); err != nil {
			return nil, err
		}
		return result, nil
	case valueType.Is(tftypes.List{}) || valueType.Is(tftypes.Set{}) || valueType.Is(tftypes.Tuple{}):
		var values []tftypes.Value
		if err := value.As(&values); err != nil {
			return nil, err
		}

		result := make([]any, 0, len(values))
		for _, item := range values {
			converted, err := terraformValueToJSONValue(item)
			if err != nil {
				return nil, err
			}
			result = append(result, converted)
		}
		return result, nil
	case valueType.Is(tftypes.Map{}) || valueType.Is(tftypes.Object{}):
		var values map[string]tftypes.Value
		if err := value.As(&values); err != nil {
			return nil, err
		}

		result := make(map[string]any, len(values))
		for key, item := range values {
			converted, err := terraformValueToJSONValue(item)
			if err != nil {
				return nil, err
			}
			result[key] = converted
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported config value type %s", valueType)
	}
}
