package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestProviderSchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	resp := &fwprovider.SchemaResponse{}
	New("0.1.0")().Schema(ctx, fwprovider.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema: %+v", resp.Diagnostics)
	}
	diags := resp.Schema.ValidateImplementation(ctx)
	if diags.HasError() {
		t.Fatalf("ValidateImplementation: %+v", diags)
	}
}

func TestResourceSchemas(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	resources := []func() resource.Resource{
		NewUnitResource,
		NewTimerResource,
		NewMountResource,
		NewAutomountResource,
		NewSocketResource,
		NewTargetResource,
		NewDropinResource,
		NewNetworkResource,
		NewNetdevResource,
		NewLinkResource,
		NewInstanceResource,
	}
	for _, neo := range resources {
		r := neo()
		t.Run(typeNameOf(ctx, r), func(t *testing.T) {
			t.Parallel()
			resp := &resource.SchemaResponse{}
			r.Schema(ctx, resource.SchemaRequest{}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("Schema: %+v", resp.Diagnostics)
			}
			diags := resp.Schema.ValidateImplementation(ctx)
			if diags.HasError() {
				t.Fatalf("ValidateImplementation: %+v", diags)
			}
		})
	}
}

func TestDataSourceSchemas(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sources := []func() datasource.DataSource{
		NewUnitDataSource,
		NewLinkDataSource,
	}
	for _, neo := range sources {
		d := neo()
		t.Run(dsTypeNameOf(ctx, d), func(t *testing.T) {
			t.Parallel()
			resp := &datasource.SchemaResponse{}
			d.Schema(ctx, datasource.SchemaRequest{}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("Schema: %+v", resp.Diagnostics)
			}
			diags := resp.Schema.ValidateImplementation(ctx)
			if diags.HasError() {
				t.Fatalf("ValidateImplementation: %+v", diags)
			}
		})
	}
}

func typeNameOf(ctx context.Context, r resource.Resource) string {
	resp := &resource.MetadataResponse{}
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "systemd"}, resp)
	return resp.TypeName
}

func dsTypeNameOf(ctx context.Context, d datasource.DataSource) string {
	resp := &datasource.MetadataResponse{}
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "systemd"}, resp)
	return resp.TypeName
}
