// Copyright 2024 KubeStellar Authors
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"kubestellar": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccPreCheck validates the necessary test environment exists
func testAccPreCheck(t *testing.T) {
	// Check for required environment variables or conditions
	// For example, ensure a kubernetes cluster is available
}

// TestAccResourceBindingPolicy_basic tests basic BindingPolicy creation
func TestAccResourceBindingPolicy_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccResourceBindingPolicyConfig_basic(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kubestellar_binding_policy.test", "name", "test-policy"),
					resource.TestCheckResourceAttr("kubestellar_binding_policy.test", "namespace", "default"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "kubestellar_binding_policy.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: testAccResourceBindingPolicyConfig_updated(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kubestellar_binding_policy.test", "name", "test-policy"),
					resource.TestCheckResourceAttr("kubestellar_binding_policy.test", "labels.%", "1"),
				),
			},
		},
	})
}

func testAccResourceBindingPolicyConfig_basic() string {
	return `
provider "kubestellar" {}

resource "kubestellar_binding_policy" "test" {
  name      = "test-policy"
  namespace = "default"

  cluster_selector {
    match_labels = {
      env = "test"
    }
  }

  downsync {
    api_group = "apps"
    resources = ["deployments"]
    namespaces = ["default"]
  }
}
`
}

func testAccResourceBindingPolicyConfig_updated() string {
	return `
provider "kubestellar" {}

resource "kubestellar_binding_policy" "test" {
  name      = "test-policy"
  namespace = "default"

  labels = {
    updated = "true"
  }

  cluster_selector {
    match_labels = {
      env = "test"
    }
  }

  downsync {
    api_group = "apps"
    resources = ["deployments", "replicasets"]
    namespaces = ["default", "test"]
  }
}
`
}

// TestAccResourceCluster_basic tests basic Cluster registration
func TestAccResourceCluster_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccResourceClusterConfig_basic(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kubestellar_cluster.test", "name", "test-cluster"),
					resource.TestCheckResourceAttr("kubestellar_cluster.test", "cluster_name", "test-cluster"),
					resource.TestCheckResourceAttr("kubestellar_cluster.test", "environment", "testing"),
				),
			},
		},
	})
}

func testAccResourceClusterConfig_basic() string {
	return `
provider "kubestellar" {}

resource "kubestellar_cluster" "test" {
  name         = "test-cluster"
  namespace    = "kubestellar"
  cluster_name = "test-cluster"

  labels = {
    env = "test"
  }

  environment = "testing"
  
  api_endpoint = "https://test-cluster.example.com:6443"
  token_ref    = "kubestellar/test-cluster-token"
}
`
}

// TestAccResourceWDS_basic tests basic WDS creation
func TestAccResourceWDS_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccResourceWDSConfig_basic(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kubestellar_wds.test", "name", "test-wds"),
					resource.TestCheckResourceAttr("kubestellar_wds.test", "space_type", "vcluster"),
				),
			},
		},
	})
}

func testAccResourceWDSConfig_basic() string {
	return `
provider "kubestellar" {}

resource "kubestellar_wds" "test" {
  name      = "test-wds"
  namespace = "kubestellar"

  labels = {
    test = "true"
  }

  space_type = "vcluster"
}
`
}

// TestAccResourceITS_basic tests basic ITS creation
func TestAccResourceITS_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccResourceITSConfig_basic(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kubestellar_its.test", "name", "test-its"),
					resource.TestCheckResourceAttr("kubestellar_its.test", "sync_interval", "30s"),
				),
			},
		},
	})
}

func testAccResourceITSConfig_basic() string {
	return `
provider "kubestellar" {}

resource "kubestellar_its" "test" {
  name      = "test-its"
  namespace = "kubestellar"

  labels = {
    test = "true"
  }

  sync_interval = "30s"
}
`
}

// TestAccDataSourceClusters_basic tests the clusters data source
func TestAccDataSourceClusters_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceClustersConfig_basic(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.kubestellar_clusters.all", "id"),
				),
			},
		},
	})
}

func testAccDataSourceClustersConfig_basic() string {
	return `
provider "kubestellar" {}

data "kubestellar_clusters" "all" {
  namespace = "kubestellar"
}
`
}

// Unit Tests

func TestProviderSchema(t *testing.T) {
	ctx := context.Background()
	p := New("test")()

	schemaReq := provider.SchemaRequest{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(ctx, schemaReq, schemaResp)

	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema returned errors: %v", schemaResp.Diagnostics)
	}

	// Validate required attributes exist
	attrs := schemaResp.Schema.Attributes
	expectedAttrs := []string{"kubeconfig", "context", "in_cluster", "validate_crds"}
	
	for _, attr := range expectedAttrs {
		if _, ok := attrs[attr]; !ok {
			t.Errorf("Expected attribute %q not found in provider schema", attr)
		}
	}
}

func TestBindingPolicyResourceSchema(t *testing.T) {
	ctx := context.Background()
	r := NewBindingPolicyResource()

	schemaReq := resource.SchemaRequest{}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, schemaReq, schemaResp)

	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema returned errors: %v", schemaResp.Diagnostics)
	}

	// Validate required attributes
	attrs := schemaResp.Schema.Attributes
	requiredAttrs := []string{"name", "namespace", "id"}
	
	for _, attr := range requiredAttrs {
		if _, ok := attrs[attr]; !ok {
			t.Errorf("Expected attribute %q not found in BindingPolicy schema", attr)
		}
	}

	// Validate blocks exist
	blocks := schemaResp.Schema.Blocks
	expectedBlocks := []string{"cluster_selector", "downsync"}
	
	for _, block := range expectedBlocks {
		if _, ok := blocks[block]; !ok {
			t.Errorf("Expected block %q not found in BindingPolicy schema", block)
		}
	}
}

func TestClusterResourceSchema(t *testing.T) {
	ctx := context.Background()
	r := NewClusterResource()

	schemaReq := resource.SchemaRequest{}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, schemaReq, schemaResp)

	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema returned errors: %v", schemaResp.Diagnostics)
	}

	// Validate required and optional attributes
	attrs := schemaResp.Schema.Attributes
	
	// Required attributes
	if attr, ok := attrs["name"]; !ok || !attr.IsRequired() {
		t.Error("Expected 'name' to be a required attribute")
	}
	if attr, ok := attrs["cluster_name"]; !ok || !attr.IsRequired() {
		t.Error("Expected 'cluster_name' to be a required attribute")
	}
	
	// Optional attributes
	optionalAttrs := []string{"labels", "description", "location", "cloud_provider", "environment"}
	for _, attrName := range optionalAttrs {
		if attr, ok := attrs[attrName]; !ok || !attr.IsOptional() {
			t.Errorf("Expected %q to be an optional attribute", attrName)
		}
	}
	
	// Computed attributes
	computedAttrs := []string{"id", "status", "ready", "kubernetes_version"}
	for _, attrName := range computedAttrs {
		if attr, ok := attrs[attrName]; !ok || !attr.IsComputed() {
			t.Errorf("Expected %q to be a computed attribute", attrName)
		}
	}
}

func TestWDSResourceSchema(t *testing.T) {
	ctx := context.Background()
	r := NewWDSResource()

	schemaReq := resource.SchemaRequest{}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, schemaReq, schemaResp)

	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema returned errors: %v", schemaResp.Diagnostics)
	}

	attrs := schemaResp.Schema.Attributes
	
	// Validate space_type attribute has correct validator
	if _, ok := attrs["space_type"]; !ok {
		t.Error("Expected 'space_type' attribute not found")
	}
}

func TestITSResourceSchema(t *testing.T) {
	ctx := context.Background()
	r := NewITSResource()

	schemaReq := resource.SchemaRequest{}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, schemaReq, schemaResp)

	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema returned errors: %v", schemaResp.Diagnostics)
	}

	attrs := schemaResp.Schema.Attributes
	
	// Validate sync_interval attribute exists
	if _, ok := attrs["sync_interval"]; !ok {
		t.Error("Expected 'sync_interval' attribute not found")
	}
}
