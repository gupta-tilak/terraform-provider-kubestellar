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
var _ resource.Resource = &WDSResource{}
var _ resource.ResourceWithImportState = &WDSResource{}
var _ resource.ResourceWithValidateConfig = &WDSResource{}

// WDSResource defines the resource implementation for Workload Definition Space.
type WDSResource struct {
	providerData *ProviderData
}

// NewWDSResource creates a new WDS resource
func NewWDSResource() resource.Resource {
	return &WDSResource{}
}

// Metadata returns the resource type name
func (r *WDSResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_wds"
}

// Schema defines the schema for the WDS resource
func (r *WDSResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a KubeStellar Workload Definition Space (WDS). " +
			"WDS is a Kubernetes-compatible API server where users apply workloads and policies.",
		MarkdownDescription: `
# kubestellar_wds Resource

Manages a KubeStellar **Workload Definition Space (WDS)**.

## What is WDS?

The Workload Definition Space is a Kubernetes-compatible API server that serves as 
the primary control plane for KubeStellar. Users apply their workloads (Deployments, 
Services, ConfigMaps, etc.) and BindingPolicies to the WDS, and KubeStellar 
distributes them to the appropriate Workload Execution Clusters (WECs).

## Key Features

- Standard Kubernetes API surface for workload definitions
- Stores BindingPolicy CRDs for workload-to-cluster binding
- Integrates with GitOps tools (Argo CD, Flux)
- Provides status aggregation from WECs

## Example Usage

` + "```hcl" + `
resource "kubestellar_wds" "main" {
  name      = "main-wds"
  namespace = "kubestellar"

  labels = {
    environment = "production"
  }

  space_type   = "vcluster"
  api_endpoint = "https://wds.example.com:6443"
}
` + "```" + `
`,
		Attributes: map[string]schema.Attribute{
			// Identity fields
			"id": schema.StringAttribute{
				Description: "Unique identifier for the WDS resource.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the WDS. Must be unique within the namespace.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 253),
				},
			},
			"namespace": schema.StringAttribute{
				Description: "Namespace where the WDS is created. Defaults to 'kubestellar'.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("kubestellar"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"labels": schema.MapAttribute{
				Description: "Labels to apply to the WDS resource.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"annotations": schema.MapAttribute{
				Description: "Annotations to apply to the WDS resource.",
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
				Description: "API server endpoint URL for the WDS. Required for 'external' space type.",
				Optional:    true,
			},
			"ca_data": schema.StringAttribute{
				Description: "PEM-encoded CA certificate data for TLS verification.",
				Optional:    true,
				Sensitive:   true,
			},
			"token_ref": schema.StringAttribute{
				Description: "Reference to a Secret containing the authentication token. " +
					"Format: 'namespace/secret-name'.",
				Optional: true,
			},

			// Status fields (computed)
			"status": schema.StringAttribute{
				Description: "Current status of the WDS (e.g., 'Ready', 'Provisioning', 'Error').",
				Computed:    true,
			},
			"ready": schema.BoolAttribute{
				Description: "Whether the WDS is ready to accept workloads.",
				Computed:    true,
			},
		},
	}
}

// ValidateConfig validates the resource configuration
func (r *WDSResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data WDSResourceModel

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
func (r *WDSResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create creates a new WDS resource
func (r *WDSResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data WDSResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating WDS", map[string]interface{}{
		"name":      data.Name.ValueString(),
		"namespace": data.Namespace.ValueString(),
	})

	// Build the WDS custom resource
	wds := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "control.kubestellar.io/v1alpha1",
			"kind":       "WorkloadDefinitionSpace",
			"metadata": map[string]interface{}{
				"name":      data.Name.ValueString(),
				"namespace": data.Namespace.ValueString(),
			},
			"spec": map[string]interface{}{
				"spaceType": data.SpaceType.ValueString(),
			},
		},
	}

	// Add optional fields
	metadata := wds.Object["metadata"].(map[string]interface{})
	spec := wds.Object["spec"].(map[string]interface{})

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

	// Create the WDS resource
	gvr := k8sschema.GroupVersionResource{
		Group:    "control.kubestellar.io",
		Version:  "v1alpha1",
		Resource: "workloaddefinitionspaces",
	}

	created, err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Create(ctx, wds, metav1.CreateOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to create WDS",
			fmt.Sprintf("Could not create WDS %s: %s", data.Name.ValueString(), err),
		)
		return
	}

	// Set computed values
	data.ID = types.StringValue(fmt.Sprintf("%s/%s", data.Namespace.ValueString(), data.Name.ValueString()))
	data.Status = types.StringValue("Provisioning")
	data.Ready = types.BoolValue(false)

	// Extract status if available
	if status, ok := created.Object["status"].(map[string]interface{}); ok {
		if phase, ok := status["phase"].(string); ok {
			data.Status = types.StringValue(phase)
		}
		if ready, ok := status["ready"].(bool); ok {
			data.Ready = types.BoolValue(ready)
		}
	}

	tflog.Info(ctx, "Created WDS", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read refreshes the Terraform state with the latest data
func (r *WDSResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data WDSResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading WDS", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	gvr := k8sschema.GroupVersionResource{
		Group:    "control.kubestellar.io",
		Version:  "v1alpha1",
		Resource: "workloaddefinitionspaces",
	}

	wds, err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Get(ctx, data.Name.ValueString(), metav1.GetOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to read WDS",
			fmt.Sprintf("Could not read WDS %s: %s", data.Name.ValueString(), err),
		)
		return
	}

	// Update data from the fetched resource
	metadata := wds.Object["metadata"].(map[string]interface{})
	spec := wds.Object["spec"].(map[string]interface{})

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

	if apiEndpoint, ok := spec["apiEndpoint"].(string); ok {
		data.APIEndpoint = types.StringValue(apiEndpoint)
	}

	// Update status
	if status, ok := wds.Object["status"].(map[string]interface{}); ok {
		if phase, ok := status["phase"].(string); ok {
			data.Status = types.StringValue(phase)
		}
		if ready, ok := status["ready"].(bool); ok {
			data.Ready = types.BoolValue(ready)
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update updates the WDS resource
func (r *WDSResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data WDSResourceModel
	var state WDSResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating WDS", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	gvr := k8sschema.GroupVersionResource{
		Group:    "control.kubestellar.io",
		Version:  "v1alpha1",
		Resource: "workloaddefinitionspaces",
	}

	// Get current resource
	current, err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Get(ctx, data.Name.ValueString(), metav1.GetOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to get WDS for update",
			fmt.Sprintf("Could not get WDS %s: %s", data.Name.ValueString(), err),
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

	if !data.APIEndpoint.IsNull() {
		spec["apiEndpoint"] = data.APIEndpoint.ValueString()
	}

	if !data.CAData.IsNull() {
		spec["caData"] = data.CAData.ValueString()
	}

	if !data.TokenRef.IsNull() {
		spec["tokenRef"] = data.TokenRef.ValueString()
	}

	// Update the resource
	updated, err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Update(ctx, current, metav1.UpdateOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to update WDS",
			fmt.Sprintf("Could not update WDS %s: %s", data.Name.ValueString(), err),
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
	}

	// Preserve ID
	data.ID = state.ID

	tflog.Info(ctx, "Updated WDS", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete deletes the WDS resource
func (r *WDSResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data WDSResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting WDS", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	gvr := k8sschema.GroupVersionResource{
		Group:    "control.kubestellar.io",
		Version:  "v1alpha1",
		Resource: "workloaddefinitionspaces",
	}

	err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Delete(ctx, data.Name.ValueString(), metav1.DeleteOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to delete WDS",
			fmt.Sprintf("Could not delete WDS %s: %s", data.Name.ValueString(), err),
		)
		return
	}

	// Wait for deletion to complete
	for i := 0; i < 30; i++ {
		_, err := r.providerData.DynamicClient.Resource(gvr).
			Namespace(data.Namespace.ValueString()).
			Get(ctx, data.Name.ValueString(), metav1.GetOptions{})
		if err != nil {
			break // Resource is deleted
		}
		time.Sleep(2 * time.Second)
	}

	tflog.Info(ctx, "Deleted WDS", map[string]interface{}{
		"id": data.ID.ValueString(),
	})
}

// ImportState imports an existing WDS resource
func (r *WDSResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID format: namespace/name
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
