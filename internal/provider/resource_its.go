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
var _ resource.Resource = &ITSResource{}
var _ resource.ResourceWithImportState = &ITSResource{}
var _ resource.ResourceWithValidateConfig = &ITSResource{}

// ITSResource defines the resource implementation for Inventory and Transport Space.
type ITSResource struct {
	providerData *ProviderData
}

// NewITSResource creates a new ITS resource
func NewITSResource() resource.Resource {
	return &ITSResource{}
}

// Metadata returns the resource type name
func (r *ITSResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_its"
}

// Schema defines the schema for the ITS resource
func (r *ITSResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a KubeStellar Inventory and Transport Space (ITS). " +
			"ITS tracks clusters and their metadata, enabling cluster inventory management.",
		MarkdownDescription: `
# kubestellar_its Resource

Manages a KubeStellar **Inventory and Transport Space (ITS)**.

## What is ITS?

The Inventory and Transport Space is the central registry for all Workload 
Execution Clusters (WECs) managed by KubeStellar. It:

- Maintains an inventory of all registered clusters
- Stores cluster metadata and labels for selection
- Handles transport of workloads to target clusters
- Aggregates status from WECs back to WDS

## Key Features

- Cluster registration and discovery
- Label-based cluster organization
- Transport layer for multi-cluster communication
- Status aggregation and reporting

## Example Usage

` + "```hcl" + `
resource "kubestellar_its" "main" {
  name      = "main-its"
  namespace = "kubestellar"

  labels = {
    environment = "production"
  }

  space_type    = "vcluster"
  sync_interval = "30s"
}
` + "```" + `
`,
		Attributes: map[string]schema.Attribute{
			// Identity fields
			"id": schema.StringAttribute{
				Description: "Unique identifier for the ITS resource.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the ITS. Must be unique within the namespace.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 253),
				},
			},
			"namespace": schema.StringAttribute{
				Description: "Namespace where the ITS is created. Defaults to 'kubestellar'.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("kubestellar"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"labels": schema.MapAttribute{
				Description: "Labels to apply to the ITS resource.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"annotations": schema.MapAttribute{
				Description: "Annotations to apply to the ITS resource.",
				Optional:    true,
				ElementType: types.StringType,
			},

			// Spec fields
			"space_type": schema.StringAttribute{
				Description: "Type of the space implementation. Options: 'vcluster', 'kind', 'external'. " +
					"Immutable after creation.",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("vcluster"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("vcluster", "kind", "external"),
				},
			},
			"api_endpoint": schema.StringAttribute{
				Description: "API server endpoint URL for the ITS. Required for 'external' space type.",
				Optional:    true,
			},
			"ca_data": schema.StringAttribute{
				Description: "PEM-encoded CA certificate data for TLS verification.",
				Optional:    true,
				Sensitive:   true,
			},
			"token_ref": schema.StringAttribute{
				Description: "Reference to a Secret containing the authentication token.",
				Optional:    true,
			},
			"sync_interval": schema.StringAttribute{
				Description: "How often to sync cluster inventory. Format: duration (e.g., '30s', '1m').",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("30s"),
			},

			// Status fields (computed)
			"status": schema.StringAttribute{
				Description: "Current status of the ITS.",
				Computed:    true,
			},
			"ready": schema.BoolAttribute{
				Description: "Whether the ITS is ready.",
				Computed:    true,
			},
			"cluster_count": schema.Int64Attribute{
				Description: "Number of clusters registered in this ITS.",
				Computed:    true,
			},
		},
	}
}

// ValidateConfig validates the resource configuration
func (r *ITSResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data ITSResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate that external space_type requires api_endpoint
	if !data.SpaceType.IsNull() && data.SpaceType.ValueString() == "external" {
		if data.APIEndpoint.IsNull() || data.APIEndpoint.ValueString() == "" {
			resp.Diagnostics.AddAttributeError(
				path.Root("api_endpoint"),
				"Missing required attribute",
				"api_endpoint is required when space_type is 'external'",
			)
		}
	}
}

// Configure sets up the resource with provider data
func (r *ITSResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create creates a new ITS resource
func (r *ITSResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ITSResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating ITS", map[string]interface{}{
		"name":      data.Name.ValueString(),
		"namespace": data.Namespace.ValueString(),
	})

	// Build the ITS custom resource
	its := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "control.kubestellar.io/v1alpha1",
			"kind":       "InventoryTransportSpace",
			"metadata": map[string]interface{}{
				"name":      data.Name.ValueString(),
				"namespace": data.Namespace.ValueString(),
			},
			"spec": map[string]interface{}{
				"spaceType":    data.SpaceType.ValueString(),
				"syncInterval": data.SyncInterval.ValueString(),
			},
		},
	}

	// Add optional fields
	metadata := its.Object["metadata"].(map[string]interface{})
	spec := its.Object["spec"].(map[string]interface{})

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

	if !data.APIEndpoint.IsNull() {
		spec["apiEndpoint"] = data.APIEndpoint.ValueString()
	}

	if !data.CAData.IsNull() {
		spec["caData"] = data.CAData.ValueString()
	}

	if !data.TokenRef.IsNull() {
		spec["tokenRef"] = data.TokenRef.ValueString()
	}

	// Create the ITS resource
	gvr := k8sschema.GroupVersionResource{
		Group:    "control.kubestellar.io",
		Version:  "v1alpha1",
		Resource: "inventorytransportspaces",
	}

	created, err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Create(ctx, its, metav1.CreateOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to create ITS",
			fmt.Sprintf("Could not create ITS %s: %s", data.Name.ValueString(), err),
		)
		return
	}

	// Set computed values
	data.ID = types.StringValue(fmt.Sprintf("%s/%s", data.Namespace.ValueString(), data.Name.ValueString()))
	data.Status = types.StringValue("Provisioning")
	data.Ready = types.BoolValue(false)
	data.ClusterCount = types.Int64Value(0)

	// Extract status if available
	if status, ok := created.Object["status"].(map[string]interface{}); ok {
		if phase, ok := status["phase"].(string); ok {
			data.Status = types.StringValue(phase)
		}
		if ready, ok := status["ready"].(bool); ok {
			data.Ready = types.BoolValue(ready)
		}
		if count, ok := status["clusterCount"].(int64); ok {
			data.ClusterCount = types.Int64Value(count)
		}
	}

	tflog.Info(ctx, "Created ITS", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read refreshes the Terraform state with the latest data
func (r *ITSResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ITSResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading ITS", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	gvr := k8sschema.GroupVersionResource{
		Group:    "control.kubestellar.io",
		Version:  "v1alpha1",
		Resource: "inventorytransportspaces",
	}

	its, err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Get(ctx, data.Name.ValueString(), metav1.GetOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to read ITS",
			fmt.Sprintf("Could not read ITS %s: %s", data.Name.ValueString(), err),
		)
		return
	}

	// Update data from the fetched resource
	metadata := its.Object["metadata"].(map[string]interface{})
	spec := its.Object["spec"].(map[string]interface{})

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

	if spaceType, ok := spec["spaceType"].(string); ok {
		data.SpaceType = types.StringValue(spaceType)
	}

	if syncInterval, ok := spec["syncInterval"].(string); ok {
		data.SyncInterval = types.StringValue(syncInterval)
	}

	// Update status
	if status, ok := its.Object["status"].(map[string]interface{}); ok {
		if phase, ok := status["phase"].(string); ok {
			data.Status = types.StringValue(phase)
		}
		if ready, ok := status["ready"].(bool); ok {
			data.Ready = types.BoolValue(ready)
		}
		if count, ok := status["clusterCount"].(int64); ok {
			data.ClusterCount = types.Int64Value(count)
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update updates the ITS resource
func (r *ITSResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ITSResourceModel
	var state ITSResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating ITS", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	gvr := k8sschema.GroupVersionResource{
		Group:    "control.kubestellar.io",
		Version:  "v1alpha1",
		Resource: "inventorytransportspaces",
	}

	// Get current resource
	current, err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Get(ctx, data.Name.ValueString(), metav1.GetOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to get ITS for update",
			fmt.Sprintf("Could not get ITS %s: %s", data.Name.ValueString(), err),
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

	if !data.SyncInterval.IsNull() {
		spec["syncInterval"] = data.SyncInterval.ValueString()
	}

	// Update the resource
	updated, err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Update(ctx, current, metav1.UpdateOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to update ITS",
			fmt.Sprintf("Could not update ITS %s: %s", data.Name.ValueString(), err),
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
		if count, ok := status["clusterCount"].(int64); ok {
			data.ClusterCount = types.Int64Value(count)
		}
	}

	data.ID = state.ID

	tflog.Info(ctx, "Updated ITS", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete deletes the ITS resource
func (r *ITSResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ITSResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting ITS", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	gvr := k8sschema.GroupVersionResource{
		Group:    "control.kubestellar.io",
		Version:  "v1alpha1",
		Resource: "inventorytransportspaces",
	}

	err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Delete(ctx, data.Name.ValueString(), metav1.DeleteOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to delete ITS",
			fmt.Sprintf("Could not delete ITS %s: %s", data.Name.ValueString(), err),
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

	tflog.Info(ctx, "Deleted ITS", map[string]interface{}{
		"id": data.ID.ValueString(),
	})
}

// ImportState imports an existing ITS resource
func (r *ITSResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
