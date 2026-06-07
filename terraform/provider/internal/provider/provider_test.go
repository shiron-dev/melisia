package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/shiron-dev/melisia/terraform-provider-truenas/internal/client"
)

func TestProviderMetadata(t *testing.T) {
	truenasProvider := &TrueNASProvider{version: "test-version"}

	var resp provider.MetadataResponse
	truenasProvider.Metadata(context.Background(), provider.MetadataRequest{}, &resp)

	if resp.TypeName != "truenas" {
		t.Fatalf("got type name %q, want truenas", resp.TypeName)
	}
	if resp.Version != "test-version" {
		t.Fatalf("got version %q, want test-version", resp.Version)
	}
}

func TestProviderSchema(t *testing.T) {
	truenasProvider := &TrueNASProvider{}

	var resp provider.SchemaResponse
	truenasProvider.Schema(context.Background(), provider.SchemaRequest{}, &resp)

	assertProviderAttribute(t, resp.Schema.Attributes, "base_url")
	assertProviderAttribute(t, resp.Schema.Attributes, "api_key")
	assertProviderAttribute(t, resp.Schema.Attributes, "tls_insecure_skip_verify")

	apiKey, ok := resp.Schema.Attributes["api_key"].(providerschema.StringAttribute)
	if !ok {
		t.Fatalf("api_key attribute has type %T, want StringAttribute", resp.Schema.Attributes["api_key"])
	}
	if !apiKey.Sensitive {
		t.Fatal("api_key attribute must be sensitive")
	}
}

func TestProviderResourcesAndDataSources(t *testing.T) {
	truenasProvider := &TrueNASProvider{}

	resources := truenasProvider.Resources(context.Background())
	if len(resources) != 1 {
		t.Fatalf("got %d resources, want 1", len(resources))
	}
	if _, ok := resources[0]().(*datasetResource); !ok {
		t.Fatalf("got resource %T, want *datasetResource", resources[0]())
	}

	dataSources := truenasProvider.DataSources(context.Background())
	if len(dataSources) != 1 {
		t.Fatalf("got %d data sources, want 1", len(dataSources))
	}
	if _, ok := dataSources[0]().(*poolDataSource); !ok {
		t.Fatalf("got data source %T, want *poolDataSource", dataSources[0]())
	}
}

func TestDatasetResourceMetadataAndSchema(t *testing.T) {
	dataset := &datasetResource{}

	var metadataResp resource.MetadataResponse
	dataset.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "truenas"}, &metadataResp)
	if metadataResp.TypeName != "truenas_dataset" {
		t.Fatalf("got type name %q, want truenas_dataset", metadataResp.TypeName)
	}

	var schemaResp resource.SchemaResponse
	dataset.Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	required := []string{
		"name",
		"type",
		"atime",
		"compression",
		"deduplication",
		"exec",
		"readonly",
		"recordsize",
		"sync",
	}
	for _, name := range required {
		attr, ok := schemaResp.Schema.Attributes[name]
		if !ok {
			t.Fatalf("missing dataset attribute %q", name)
		}
		if !attr.IsRequired() {
			t.Fatalf("dataset attribute %q must be required", name)
		}
	}

	id, ok := schemaResp.Schema.Attributes["id"]
	if !ok {
		t.Fatal("missing dataset id attribute")
	}
	if !id.IsComputed() {
		t.Fatal("dataset id must be computed")
	}
}

func TestDatasetResourceConfigure(t *testing.T) {
	t.Run("nil provider data is accepted", func(t *testing.T) {
		dataset := &datasetResource{}

		var resp resource.ConfigureResponse
		dataset.Configure(context.Background(), resource.ConfigureRequest{}, &resp)

		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
		}
		if dataset.client != nil {
			t.Fatal("client should remain nil")
		}
	})

	t.Run("client provider data is accepted", func(t *testing.T) {
		truenasClient := newProviderTestClient(t)
		dataset := &datasetResource{}

		var resp resource.ConfigureResponse
		dataset.Configure(context.Background(), resource.ConfigureRequest{ProviderData: truenasClient}, &resp)

		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
		}
		if dataset.client != truenasClient {
			t.Fatal("client was not configured")
		}
	})

	t.Run("wrong provider data type returns diagnostic", func(t *testing.T) {
		dataset := &datasetResource{}

		var resp resource.ConfigureResponse
		dataset.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, &resp)

		assertDiagnosticSummary(t, resp.Diagnostics, "Unexpected resource configure type")
	})
}

func TestPoolDataSourceMetadataAndSchema(t *testing.T) {
	pool := &poolDataSource{}

	var metadataResp datasource.MetadataResponse
	pool.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "truenas"}, &metadataResp)
	if metadataResp.TypeName != "truenas_pool" {
		t.Fatalf("got type name %q, want truenas_pool", metadataResp.TypeName)
	}

	var schemaResp datasource.SchemaResponse
	pool.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)

	id, ok := schemaResp.Schema.Attributes["id"]
	if !ok {
		t.Fatal("missing pool id attribute")
	}
	if !id.IsRequired() {
		t.Fatal("pool id must be required")
	}

	computed := []string{"name", "path", "status", "healthy", "size", "available"}
	for _, name := range computed {
		attr, ok := schemaResp.Schema.Attributes[name]
		if !ok {
			t.Fatalf("missing pool attribute %q", name)
		}
		if !attr.IsComputed() {
			t.Fatalf("pool attribute %q must be computed", name)
		}
	}
}

func TestPoolDataSourceConfigure(t *testing.T) {
	t.Run("nil provider data is accepted", func(t *testing.T) {
		pool := &poolDataSource{}

		var resp datasource.ConfigureResponse
		pool.Configure(context.Background(), datasource.ConfigureRequest{}, &resp)

		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
		}
		if pool.client != nil {
			t.Fatal("client should remain nil")
		}
	})

	t.Run("client provider data is accepted", func(t *testing.T) {
		truenasClient := newProviderTestClient(t)
		pool := &poolDataSource{}

		var resp datasource.ConfigureResponse
		pool.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: truenasClient}, &resp)

		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
		}
		if pool.client != truenasClient {
			t.Fatal("client was not configured")
		}
	})

	t.Run("wrong provider data type returns diagnostic", func(t *testing.T) {
		pool := &poolDataSource{}

		var resp datasource.ConfigureResponse
		pool.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, &resp)

		assertDiagnosticSummary(t, resp.Diagnostics, "Unexpected data source configure type")
	})
}

func TestModelToDatasetUsesNameAsIDFallback(t *testing.T) {
	got := modelToDataset(datasetResourceModel{
		Name:          types.StringValue("apps/apps"),
		Type:          types.StringValue("FILESYSTEM"),
		Atime:         types.StringValue("ON"),
		Compression:   types.StringValue("LZ4"),
		Deduplication: types.StringValue("OFF"),
		Exec:          types.StringValue("ON"),
		Readonly:      types.StringValue("OFF"),
		Recordsize:    types.StringValue("128K"),
		Sync:          types.StringValue("STANDARD"),
	})

	want := client.Dataset{
		ID:            "apps/apps",
		Name:          "apps/apps",
		Type:          "FILESYSTEM",
		Atime:         "ON",
		Compression:   "LZ4",
		Deduplication: "OFF",
		Exec:          "ON",
		Readonly:      "OFF",
		Recordsize:    "128K",
		Sync:          "STANDARD",
	}
	if got != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestModelToDatasetKeepsExplicitID(t *testing.T) {
	got := modelToDataset(datasetResourceModel{
		ID:   types.StringValue("imported/id"),
		Name: types.StringValue("apps/apps"),
	})

	if got.ID != "imported/id" {
		t.Fatalf("got ID %q, want imported/id", got.ID)
	}
}

func TestDatasetToModelUsesAPIFactsBeforeFallback(t *testing.T) {
	fallback := datasetResourceModel{
		ID:            types.StringValue("fallback-id"),
		Name:          types.StringValue("fallback-name"),
		Type:          types.StringValue("FILESYSTEM"),
		Atime:         types.StringValue("OFF"),
		Compression:   types.StringValue("GZIP"),
		Deduplication: types.StringValue("ON"),
		Exec:          types.StringValue("OFF"),
		Readonly:      types.StringValue("ON"),
		Recordsize:    types.StringValue("64K"),
		Sync:          types.StringValue("ALWAYS"),
	}

	got := datasetToModel(client.Dataset{
		ID:            "apps/apps",
		Name:          "apps/apps",
		Type:          "FILESYSTEM",
		Atime:         "ON",
		Compression:   "LZ4",
		Deduplication: "OFF",
		Exec:          "ON",
		Readonly:      "OFF",
		Recordsize:    "128K",
		Sync:          "STANDARD",
	}, fallback)

	if got.ID.ValueString() != "apps/apps" {
		t.Fatalf("got ID %q, want apps/apps", got.ID.ValueString())
	}
	if got.Compression.ValueString() != "LZ4" {
		t.Fatalf("got compression %q, want LZ4", got.Compression.ValueString())
	}
	if got.Recordsize.ValueString() != "128K" {
		t.Fatalf("got recordsize %q, want 128K", got.Recordsize.ValueString())
	}
}

func TestDatasetToModelUsesFallbackWhenAPILeavesFieldsEmpty(t *testing.T) {
	got := datasetToModel(client.Dataset{}, datasetResourceModel{
		ID:            types.StringValue("fallback-id"),
		Name:          types.StringValue("fallback-name"),
		Type:          types.StringValue("FILESYSTEM"),
		Atime:         types.StringValue("ON"),
		Compression:   types.StringValue("LZ4"),
		Deduplication: types.StringValue("OFF"),
		Exec:          types.StringValue("ON"),
		Readonly:      types.StringValue("OFF"),
		Recordsize:    types.StringValue("128K"),
		Sync:          types.StringValue("STANDARD"),
	})

	if got.ID.ValueString() != "fallback-id" {
		t.Fatalf("got ID %q, want fallback-id", got.ID.ValueString())
	}
	if got.Name.ValueString() != "fallback-name" {
		t.Fatalf("got name %q, want fallback-name", got.Name.ValueString())
	}
	if got.Sync.ValueString() != "STANDARD" {
		t.Fatalf("got sync %q, want STANDARD", got.Sync.ValueString())
	}
}

func TestFirstNonEmpty(t *testing.T) {
	got := firstNonEmpty("", "", "first", "second")
	if got != "first" {
		t.Fatalf("got %q, want first", got)
	}

	if firstNonEmpty("", "") != "" {
		t.Fatal("expected empty string")
	}
}

func assertProviderAttribute(t *testing.T, attributes map[string]providerschema.Attribute, name string) {
	t.Helper()

	attr, ok := attributes[name]
	if !ok {
		t.Fatalf("missing provider attribute %q", name)
	}
	if !attr.IsOptional() {
		t.Fatalf("provider attribute %q must be optional", name)
	}
}

func assertDiagnosticSummary(t *testing.T, diagnostics diag.Diagnostics, summary string) {
	t.Helper()

	if diagnostics.ErrorsCount() == 0 {
		t.Fatalf("expected diagnostic %q", summary)
	}

	for _, diagnostic := range diagnostics.Errors() {
		if strings.Contains(diagnostic.Summary(), summary) {
			return
		}
	}

	t.Fatalf("diagnostics %#v do not contain summary %q", diagnostics.Errors(), summary)
}

func newProviderTestClient(t *testing.T) *client.Client {
	t.Helper()

	truenasClient, err := client.New("https://truenas.example.test", "test-key", false)
	if err != nil {
		t.Fatal(err)
	}

	return truenasClient
}
