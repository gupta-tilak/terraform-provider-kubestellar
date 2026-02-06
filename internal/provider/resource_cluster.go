// Copyright 2024 KubeStellar Authors
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ClusterResource{}
var _ resource.ResourceWithImportState = &ClusterResource{}
var _ resource.ResourceWithValidateConfig = &ClusterResource{}

// ClusterResource defines the resource for registering WECs with KubeStellar.
type ClusterResource struct {
	providerData *ProviderData
}

// NewClusterResource creates a new Cluster resource
func NewClusterResource() resource.Resource {
	return &ClusterResource{}
}

// Metadata returns the resource type name
func (r *ClusterResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

// Schema defines the schema for the Cluster resource
func (r *ClusterResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Registers and manages a Workload Execution Cluster (WEC) with KubeStellar. " +
			"WECs are real Kubernetes clusters where workloads are deployed.",
		MarkdownDescription: `
# kubestellar_cluster Resource

Registers and manages a **Workload Execution Cluster (WEC)** with KubeStellar.

## What are WECs?

Workload Execution Clusters are the real Kubernetes clusters where your 
workloads actually run. KubeStellar distributes workloads from the WDS 
to WECs based on BindingPolicies.

## Key Features

- Register existing Kubernetes clusters with KubeStellar
- Apply labels for cluster selection via BindingPolicies
- Support for multiple cloud providers and on-premise clusters
- Automatic status monitoring and health checks

## Example Usage

` + "```hcl" + `
resource "kubestellar_cluster" "prod_us_east" {
  name        = "prod-us-east-1"
  namespace   = "kubestellar"
  cluster_name = "prod-us-east-1"

  labels = {
    env      = "production"
    region   = "us-east-1"
    provider = "aws"
  }

  description    = "Production cluster in US East 1"
  location       = "us-east-1"
  cloud_provider = "aws"
  environment    = "production"

  kubeconfig = file("~/.kube/prod-us-east-1.config")
}
` + "```" + `

## Import

Clusters can be imported using the format ` + "`namespace/name`" + `:

` + "```bash" + `
terraform import kubestellar_cluster.prod_us_east kubestellar/prod-us-east-1
` + "```" + `
`,
		Attributes: map[string]schema.Attribute{
			// Identity fields
			"id": schema.StringAttribute{
				Description: "Unique identifier for the cluster resource.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the cluster registration in KubeStellar.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 253),
				},
			},
			"namespace": schema.StringAttribute{
				Description: "Namespace where the cluster is registered. Defaults to 'kubestellar'.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("kubestellar"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"labels": schema.MapAttribute{
				Description: "Labels to apply to the cluster. Used by BindingPolicy selectors.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"annotations": schema.MapAttribute{
				Description: "Annotations to apply to the cluster resource.",
				Optional:    true,
				ElementType: types.StringType,
			},

			// Spec fields - immutable
			"cluster_name": schema.StringAttribute{
				Description: "The actual name of the Kubernetes cluster. Immutable after creation.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			// Spec fields - mutable
			"description": schema.StringAttribute{
				Description: "Human-readable description of the cluster.",
				Optional:    true,
			},

			// Connection configuration
			"kubeconfig": schema.StringAttribute{
				Description: "Kubeconfig content for connecting to the cluster. " +
					"Either kubeconfig or api_endpoint with credentials must be provided.",
				Optional:  true,
				Sensitive: true,
			},
			"api_endpoint": schema.StringAttribute{
				Description: "Kubernetes API server endpoint URL.",
				Optional:    true,
			},
			"ca_data": schema.StringAttribute{
				Description: "PEM-encoded CA certificate for TLS verification.",
				Optional:    true,
				Sensitive:   true,
			},
			"token_ref": schema.StringAttribute{
				Description: "Reference to a Secret containing authentication credentials. " +
					"Format: 'namespace/secret-name'.",
				Optional: true,
			},

			// Cluster properties
			"location": schema.StringAttribute{
				Description: "Geographic location or region of the cluster (e.g., 'us-east-1').",
				Optional:    true,
			},
			"cloud_provider": schema.StringAttribute{
				Description: "Cloud provider hosting the cluster (e.g., 'aws', 'gcp', 'azure', 'on-prem').",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("aws", "gcp", "azure", "on-prem", "kind", "k3d", "minikube", "other"),
				},
			},
			"environment": schema.StringAttribute{
				Description: "Environment type (e.g., 'production', 'staging', 'development').",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("production", "staging", "development", "testing"),
				},
			},

			// Status fields (computed)
			"status": schema.StringAttribute{
				Description: "Current status of the cluster registration (e.g., 'Ready', 'NotReady', 'Unknown').",
				Computed:    true,
			},
			"ready": schema.BoolAttribute{
				Description: "Whether the cluster is ready and connected.",
				Computed:    true,
			},
			"kubernetes_version": schema.StringAttribute{
				Description: "Detected Kubernetes version of the cluster.",
				Computed:    true,
			},
			"node_count": schema.Int64Attribute{
				Description: "Number of nodes in the cluster.",
				Computed:    true,
			},
			"last_sync_time": schema.StringAttribute{
				Description: "Last time the cluster status was synced.",
				Computed:    true,
			},
		},
	}
}

// ValidateConfig validates the resource configuration
func (r *ClusterResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data ClusterResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate that either kubeconfig or api_endpoint is provided
	if data.Kubeconfig.IsNull() && data.APIEndpoint.IsNull() {
		resp.Diagnostics.AddError(
			"Missing connection configuration",
			"Either 'kubeconfig' or 'api_endpoint' must be provided to connect to the cluster.",
		)
	}

	// If api_endpoint is provided, token_ref or ca_data should also be provided
	if !data.APIEndpoint.IsNull() && data.TokenRef.IsNull() {
		resp.Diagnostics.AddWarning(
			"Missing authentication",
			"When using 'api_endpoint', consider providing 'token_ref' for authentication.",
		)
	}
}

// Configure sets up the resource with provider data
func (r *ClusterResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *ProviderData, got: %T", req.ProviderData),
		)
		return
	}

	r.providerData = providerData
}

// Create creates a new Cluster resource
func (r *ClusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ClusterResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Registering cluster with KubeStellar", map[string]interface{}{
		"name":        data.Name.ValueString(),
		"cluster":     data.ClusterName.ValueString(),
		"namespace":   data.Namespace.ValueString(),
	})

	// Build the ManagedCluster custom resource
	cluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "control.kubestellar.io/v1alpha1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name":      data.Name.ValueString(),
				"namespace": data.Namespace.ValueString(),
			},
			"spec": map[string]interface{}{
				"clusterName": data.ClusterName.ValueString(),
			},
		},
	}

	metadata := cluster.Object["metadata"].(map[string]interface{})
	spec := cluster.Object["spec"].(map[string]interface{})

	// Add labels
	if !data.Labels.IsNull() {
		labels := make(map[string]interface{})
		for k, v := range data.Labels.Elements() {
			labels[k] = v.(types.String).ValueString()
		}
		metadata["labels"] = labels
	}

	// Add annotations
	if !data.Annotations.IsNull() {
		annotations := make(map[string]interface{})
		for k, v := range data.Annotations.Elements() {
			annotations[k] = v.(types.String).ValueString()
		}
		metadata["annotations"] = annotations
	}

	// Add optional spec fields
	if !data.Description.IsNull() {
		spec["description"] = data.Description.ValueString()
	}

	if !data.Location.IsNull() {
		spec["location"] = data.Location.ValueString()
	}

	if !data.CloudProvider.IsNull() {
		spec["cloudProvider"] = data.CloudProvider.ValueString()
	}

	if !data.Environment.IsNull() {
		spec["environment"] = data.Environment.ValueString()
	}

	// Connection configuration
	if !data.Kubeconfig.IsNull() {
		// Store kubeconfig in a secret and reference it
		spec["kubeconfigSecret"] = map[string]interface{}{
			"name":      fmt.Sprintf("%s-kubeconfig", data.Name.ValueString()),
			"namespace": data.Namespace.ValueString(),
		}
		// Create the secret with kubeconfig
		if err := r.createKubeconfigSecret(ctx, data); err != nil {
			resp.Diagnostics.AddError(
				"Failed to create kubeconfig secret",
				fmt.Sprintf("Could not create kubeconfig secret: %s", err),
			)
			return
		}
	}

	if !data.APIEndpoint.IsNull() {
		spec["apiEndpoint"] = data.APIEndpoint.ValueString()
	}

	if !data.CAData.IsNull() {
		spec["caData"] = data.CAData.ValueString()
	}

	if !data.TokenRef.IsNull() {
		spec["tokenRef"] = data.TokenRef.ValueString()
	}

	// Create the cluster resource
	gvr := k8sschema.GroupVersionResource{
		Group:    "control.kubestellar.io",
		Version:  "v1alpha1",
		Resource: "managedclusters",
	}

	created, err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Create(ctx, cluster, metav1.CreateOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to register cluster",
			fmt.Sprintf("Could not register cluster %s: %s", data.Name.ValueString(), err),
		)
		return
	}

	// Set computed values
	data.ID = types.StringValue(fmt.Sprintf("%s/%s", data.Namespace.ValueString(), data.Name.ValueString()))
	data.Status = types.StringValue("Pending")
	data.Ready = types.BoolValue(false)
	data.KubernetesVersion = types.StringNull()
	data.NodeCount = types.Int64Null()
	data.LastSyncTime = types.StringNull()

	// Extract status if available
	if status, ok := created.Object["status"].(map[string]interface{}); ok {
		if phase, ok := status["phase"].(string); ok {
			data.Status = types.StringValue(phase)
		}
		if ready, ok := status["ready"].(bool); ok {
			data.Ready = types.BoolValue(ready)
		}
		if version, ok := status["kubernetesVersion"].(string); ok {
			data.KubernetesVersion = types.StringValue(version)
		}
		if count, ok := status["nodeCount"].(int64); ok {
			data.NodeCount = types.Int64Value(count)
		}
	}

	tflog.Info(ctx, "Registered cluster with KubeStellar", map[string]interface{}{
		"id":      data.ID.ValueString(),
		"cluster": data.ClusterName.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read refreshes the Terraform state with the latest data
func (r *ClusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ClusterResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading cluster", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	gvr := k8sschema.GroupVersionResource{
		Group:    "control.kubestellar.io",
		Version:  "v1alpha1",
		Resource: "managedclusters",
	}

	cluster, err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Get(ctx, data.Name.ValueString(), metav1.GetOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to read cluster",
			fmt.Sprintf("Could not read cluster %s: %s", data.Name.ValueString(), err),
		)
		return
	}

	// Update data from the fetched resource
	metadata := cluster.Object["metadata"].(map[string]interface{})
	spec := cluster.Object["spec"].(map[string]interface{})

	data.Name = types.StringValue(metadata["name"].(string))
	data.Namespace = types.StringValue(metadata["namespace"].(string))

	if labels, ok := metadata["labels"].(map[string]interface{}); ok && len(labels) > 0 {
		labelMap := make(map[string]string)
		for k, v := range labels {
			labelMap[k] = v.(string)
		}
		labelsValue, _ := types.MapValueFrom(ctx, types.StringType, labelMap)
		data.Labels = labelsValue
	}

	if clusterName, ok := spec["clusterName"].(string); ok {
		data.ClusterName = types.StringValue(clusterName)
	}

	if description, ok := spec["description"].(string); ok {
		data.Description = types.StringValue(description)
	}

	if location, ok := spec["location"].(string); ok {
		data.Location = types.StringValue(location)
	}

	if cloudProvider, ok := spec["cloudProvider"].(string); ok {
		data.CloudProvider = types.StringValue(cloudProvider)
	}

	if environment, ok := spec["environment"].(string); ok {
		data.Environment = types.StringValue(environment)
	}

	// Update status
	if status, ok := cluster.Object["status"].(map[string]interface{}); ok {
		if phase, ok := status["phase"].(string); ok {
			data.Status = types.StringValue(phase)
		}
		if ready, ok := status["ready"].(bool); ok {
			data.Ready = types.BoolValue(ready)
		}
		if version, ok := status["kubernetesVersion"].(string); ok {
			data.KubernetesVersion = types.StringValue(version)
		}
		if count, ok := status["nodeCount"].(int64); ok {
			data.NodeCount = types.Int64Value(count)
		}
		if lastSync, ok := status["lastSyncTime"].(string); ok {
			data.LastSyncTime = types.StringValue(lastSync)
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update updates the Cluster resource
func (r *ClusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ClusterResourceModel
	var state ClusterResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating cluster", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	gvr := k8sschema.GroupVersionResource{
		Group:    "control.kubestellar.io",
		Version:  "v1alpha1",
		Resource: "managedclusters",
	}

	// Get current resource
	current, err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Get(ctx, data.Name.ValueString(), metav1.GetOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to get cluster for update",
			fmt.Sprintf("Could not get cluster %s: %s", data.Name.ValueString(), err),
		)
		return
	}

	// Update mutable fields
	metadata := current.Object["metadata"].(map[string]interface{})
	spec := current.Object["spec"].(map[string]interface{})

	if !data.Labels.IsNull() {
		labels := make(map[string]interface{})
		for k, v := range data.Labels.Elements() {
			labels[k] = v.(types.String).ValueString()
		}
		metadata["labels"] = labels
	}

	if !data.Annotations.IsNull() {
		annotations := make(map[string]interface{})
		for k, v := range data.Annotations.Elements() {
			annotations[k] = v.(types.String).ValueString()
		}
		metadata["annotations"] = annotations
	}

	if !data.Description.IsNull() {
		spec["description"] = data.Description.ValueString()
	}

	if !data.Location.IsNull() {
		spec["location"] = data.Location.ValueString()
	}

	if !data.CloudProvider.IsNull() {
		spec["cloudProvider"] = data.CloudProvider.ValueString()
	}

	if !data.Environment.IsNull() {
		spec["environment"] = data.Environment.ValueString()
	}

	// Update kubeconfig secret if changed
	if !data.Kubeconfig.IsNull() && !data.Kubeconfig.Equal(state.Kubeconfig) {
		if err := r.updateKubeconfigSecret(ctx, data); err != nil {
			resp.Diagnostics.AddError(
				"Failed to update kubeconfig secret",
				fmt.Sprintf("Could not update kubeconfig secret: %s", err),
			)
			return
		}
	}

	// Update the resource
	updated, err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Update(ctx, current, metav1.UpdateOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to update cluster",
			fmt.Sprintf("Could not update cluster %s: %s", data.Name.ValueString(), err),
		)
		return
	}

	// Update status from response
	if status, ok := updated.Object["status"].(map[string]interface{}); ok {
		if phase, ok := status["phase"].(string); ok {
			data.Status = types.StringValue(phase)
		}
		if ready, ok := status["ready"].(bool); ok {
			data.Ready = types.BoolValue(ready)
		}
		if version, ok := status["kubernetesVersion"].(string); ok {
			data.KubernetesVersion = types.StringValue(version)
		}
		if count, ok := status["nodeCount"].(int64); ok {
			data.NodeCount = types.Int64Value(count)
		}
	}

	data.ID = state.ID

	tflog.Info(ctx, "Updated cluster", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete deletes the Cluster resource
func (r *ClusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ClusterResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Unregistering cluster", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	// Delete kubeconfig secret if it exists
	if !data.Kubeconfig.IsNull() {
		r.deleteKubeconfigSecret(ctx, data)
	}

	gvr := k8sschema.GroupVersionResource{
		Group:    "control.kubestellar.io",
		Version:  "v1alpha1",
		Resource: "managedclusters",
	}

	err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Delete(ctx, data.Name.ValueString(), metav1.DeleteOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to delete cluster",
			fmt.Sprintf("Could not delete cluster %s: %s", data.Name.ValueString(), err),
		)
		return
	}

	// Wait for deletion
	for i := 0; i < 30; i++ {
		_, err := r.providerData.DynamicClient.Resource(gvr).
			Namespace(data.Namespace.ValueString()).
			Get(ctx, data.Name.ValueString(), metav1.GetOptions{})
		if err != nil {
			break
		}
		time.Sleep(2 * time.Second)
	}

	tflog.Info(ctx, "Unregistered cluster", map[string]interface{}{
		"id": data.ID.ValueString(),
	})
}

// ImportState imports an existing Cluster resource
func (r *ClusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Helper functions for kubeconfig secret management
func (r *ClusterResource) createKubeconfigSecret(ctx context.Context, data ClusterResourceModel) error {
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      fmt.Sprintf("%s-kubeconfig", data.Name.ValueString()),
				"namespace": data.Namespace.ValueString(),
			},
			"type": "Opaque",
			"stringData": map[string]interface{}{
				"kubeconfig": data.Kubeconfig.ValueString(),
			},
		},
	}

	gvr := k8sschema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "secrets",
	}

	_, err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Create(ctx, secret, metav1.CreateOptions{})
	return err
}

func (r *ClusterResource) updateKubeconfigSecret(ctx context.Context, data ClusterResourceModel) error {
	gvr := k8sschema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "secrets",
	}

	secretName := fmt.Sprintf("%s-kubeconfig", data.Name.ValueString())
	
	secret, err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		// Create if doesn't exist
		return r.createKubeconfigSecret(ctx, data)
	}

	// Update existing
	secret.Object["stringData"] = map[string]interface{}{
		"kubeconfig": data.Kubeconfig.ValueString(),
	}

	_, err = r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Update(ctx, secret, metav1.UpdateOptions{})
	return err
}

func (r *ClusterResource) deleteKubeconfigSecret(ctx context.Context, data ClusterResourceModel) {
	gvr := k8sschema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "secrets",
	}

	secretName := fmt.Sprintf("%s-kubeconfig", data.Name.ValueString())
	_ = r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Delete(ctx, secretName, metav1.DeleteOptions{})
}
