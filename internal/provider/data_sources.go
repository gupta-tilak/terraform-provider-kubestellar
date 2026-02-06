// Copyright 2024 KubeStellar Authors
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &ClustersDataSource{}

// ClustersDataSource defines the data source for listing clusters.
type ClustersDataSource struct {
	providerData *ProviderData
}

// ClustersDataSourceModel describes the data source data model.
type ClustersDataSourceModel struct {
	ID        types.String `tfsdk:"id"`
	Namespace types.String `tfsdk:"namespace"`
	Labels    types.Map    `tfsdk:"labels"`
	Clusters  types.List   `tfsdk:"clusters"`
}

// ClusterDataModel represents a single cluster in the data source
type ClusterDataModel struct {
	Name              types.String `tfsdk:"name"`
	Namespace         types.String `tfsdk:"namespace"`
	ClusterName       types.String `tfsdk:"cluster_name"`
	Labels            types.Map    `tfsdk:"labels"`
	Ready             types.Bool   `tfsdk:"ready"`
	KubernetesVersion types.String `tfsdk:"kubernetes_version"`
}

// NewClustersDataSource creates a new Clusters data source
func NewClustersDataSource() datasource.DataSource {
	return &ClustersDataSource{}
}

// Metadata returns the data source type name
func (d *ClustersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clusters"
}

// Schema defines the schema for the data source
func (d *ClustersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists KubeStellar managed clusters with optional filtering by labels.",
		MarkdownDescription: `
# kubestellar_clusters Data Source

Lists all clusters registered with KubeStellar, with optional label filtering.

## Example Usage

` + "```hcl" + `
# Get all clusters
data "kubestellar_clusters" "all" {
  namespace = "kubestellar"
}

# Get production clusters only
data "kubestellar_clusters" "prod" {
  namespace = "kubestellar"
  labels = {
    env = "production"
  }
}

output "cluster_names" {
  value = [for c in data.kubestellar_clusters.prod.clusters : c.name]
}
` + "```" + `
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Data source identifier.",
				Computed:    true,
			},
			"namespace": schema.StringAttribute{
				Description: "Namespace to list clusters from.",
				Optional:    true,
			},
			"labels": schema.MapAttribute{
				Description: "Filter clusters by labels.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"clusters": schema.ListNestedAttribute{
				Description: "List of matching clusters.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "Name of the cluster registration.",
							Computed:    true,
						},
						"namespace": schema.StringAttribute{
							Description: "Namespace of the cluster registration.",
							Computed:    true,
						},
						"cluster_name": schema.StringAttribute{
							Description: "Actual cluster name.",
							Computed:    true,
						},
						"labels": schema.MapAttribute{
							Description: "Labels on the cluster.",
							Computed:    true,
							ElementType: types.StringType,
						},
						"ready": schema.BoolAttribute{
							Description: "Whether the cluster is ready.",
							Computed:    true,
						},
						"kubernetes_version": schema.StringAttribute{
							Description: "Kubernetes version of the cluster.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

// Configure sets up the data source with provider data
func (d *ClustersDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *ProviderData, got: %T", req.ProviderData),
		)
		return
	}

	d.providerData = providerData
}

// Read refreshes the data source data
func (d *ClustersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ClustersDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading clusters data source")

	gvr := k8sschema.GroupVersionResource{
		Group:    "control.kubestellar.io",
		Version:  "v1alpha1",
		Resource: "managedclusters",
	}

	// Build label selector
	labelSelector := ""
	if !data.Labels.IsNull() {
		first := true
		for k, v := range data.Labels.Elements() {
			if !first {
				labelSelector += ","
			}
			labelSelector += fmt.Sprintf("%s=%s", k, v.(types.String).ValueString())
			first = false
		}
	}

	opts := metav1.ListOptions{
		LabelSelector: labelSelector,
	}

	var list interface{}
	var err error

	if !data.Namespace.IsNull() {
		list, err = d.providerData.DynamicClient.Resource(gvr).
			Namespace(data.Namespace.ValueString()).
			List(ctx, opts)
	} else {
		list, err = d.providerData.DynamicClient.Resource(gvr).
			List(ctx, opts)
	}

	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to list clusters",
			fmt.Sprintf("Could not list clusters: %s", err),
		)
		return
	}

	// Convert to ClusterDataModel
	clusterType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"name":               types.StringType,
			"namespace":          types.StringType,
			"cluster_name":       types.StringType,
			"labels":             types.MapType{ElemType: types.StringType},
			"ready":              types.BoolType,
			"kubernetes_version": types.StringType,
		},
	}

	var clusters []attr.Value

	listMap := list.(map[string]interface{})
	if items, ok := listMap["items"].([]interface{}); ok {
		for _, item := range items {
			itemMap := item.(map[string]interface{})
			metadata := itemMap["metadata"].(map[string]interface{})
			spec := itemMap["spec"].(map[string]interface{})

			clusterName := ""
			if cn, ok := spec["clusterName"].(string); ok {
				clusterName = cn
			}

			labels := types.MapNull(types.StringType)
			if labelsMap, ok := metadata["labels"].(map[string]interface{}); ok {
				labelStrMap := make(map[string]string)
				for k, v := range labelsMap {
					labelStrMap[k] = v.(string)
				}
				labels, _ = types.MapValueFrom(ctx, types.StringType, labelStrMap)
			}

			ready := false
			k8sVersion := ""
			if status, ok := itemMap["status"].(map[string]interface{}); ok {
				if r, ok := status["ready"].(bool); ok {
					ready = r
				}
				if v, ok := status["kubernetesVersion"].(string); ok {
					k8sVersion = v
				}
			}

			clusterObj, _ := types.ObjectValue(clusterType.AttrTypes, map[string]attr.Value{
				"name":               types.StringValue(metadata["name"].(string)),
				"namespace":          types.StringValue(metadata["namespace"].(string)),
				"cluster_name":       types.StringValue(clusterName),
				"labels":             labels,
				"ready":              types.BoolValue(ready),
				"kubernetes_version": types.StringValue(k8sVersion),
			})

			clusters = append(clusters, clusterObj)
		}
	}

	data.Clusters, _ = types.ListValue(clusterType, clusters)
	data.ID = types.StringValue("clusters")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// BindingPolicies Data Source

var _ datasource.DataSource = &BindingPoliciesDataSource{}

type BindingPoliciesDataSource struct {
	providerData *ProviderData
}

type BindingPoliciesDataSourceModel struct {
	ID        types.String `tfsdk:"id"`
	Namespace types.String `tfsdk:"namespace"`
	Policies  types.List   `tfsdk:"policies"`
}

func NewBindingPoliciesDataSource() datasource.DataSource {
	return &BindingPoliciesDataSource{}
}

func (d *BindingPoliciesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_binding_policies"
}

func (d *BindingPoliciesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists KubeStellar BindingPolicies.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"namespace": schema.StringAttribute{
				Description: "Namespace to list policies from.",
				Optional:    true,
			},
			"policies": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed: true,
						},
						"namespace": schema.StringAttribute{
							Computed: true,
						},
						"matched_clusters": schema.Int64Attribute{
							Computed: true,
						},
						"matched_workloads": schema.Int64Attribute{
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (d *BindingPoliciesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	d.providerData = req.ProviderData.(*ProviderData)
}

func (d *BindingPoliciesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data BindingPoliciesDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	gvr := k8sschema.GroupVersionResource{
		Group:    "control.kubestellar.io",
		Version:  "v1alpha1",
		Resource: "bindingpolicies",
	}

	opts := metav1.ListOptions{}

	var list interface{}
	var err error

	if !data.Namespace.IsNull() {
		list, err = d.providerData.DynamicClient.Resource(gvr).
			Namespace(data.Namespace.ValueString()).
			List(ctx, opts)
	} else {
		list, err = d.providerData.DynamicClient.Resource(gvr).
			List(ctx, opts)
	}

	if err != nil {
		resp.Diagnostics.AddError("Failed to list binding policies", err.Error())
		return
	}

	policyType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"name":              types.StringType,
			"namespace":         types.StringType,
			"matched_clusters":  types.Int64Type,
			"matched_workloads": types.Int64Type,
		},
	}

	var policies []attr.Value

	listMap := list.(map[string]interface{})
	if items, ok := listMap["items"].([]interface{}); ok {
		for _, item := range items {
			itemMap := item.(map[string]interface{})
			metadata := itemMap["metadata"].(map[string]interface{})

			matchedClusters := int64(0)
			matchedWorkloads := int64(0)
			if status, ok := itemMap["status"].(map[string]interface{}); ok {
				if mc, ok := status["matchedClusterCount"].(int64); ok {
					matchedClusters = mc
				}
				if mw, ok := status["matchedWorkloads"].(int64); ok {
					matchedWorkloads = mw
				}
			}

			policyObj, _ := types.ObjectValue(policyType.AttrTypes, map[string]attr.Value{
				"name":              types.StringValue(metadata["name"].(string)),
				"namespace":         types.StringValue(metadata["namespace"].(string)),
				"matched_clusters":  types.Int64Value(matchedClusters),
				"matched_workloads": types.Int64Value(matchedWorkloads),
			})

			policies = append(policies, policyObj)
		}
	}

	data.Policies, _ = types.ListValue(policyType, policies)
	data.ID = types.StringValue("binding_policies")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
