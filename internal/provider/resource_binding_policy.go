// Copyright 2024 KubeStellar Authors
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
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
var _ resource.Resource = &BindingPolicyResource{}
var _ resource.ResourceWithImportState = &BindingPolicyResource{}
var _ resource.ResourceWithValidateConfig = &BindingPolicyResource{}

// BindingPolicyResource defines the resource for KubeStellar BindingPolicies.
type BindingPolicyResource struct {
	providerData *ProviderData
}

// NewBindingPolicyResource creates a new BindingPolicy resource
func NewBindingPolicyResource() resource.Resource {
	return &BindingPolicyResource{}
}

// Metadata returns the resource type name
func (r *BindingPolicyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_binding_policy"
}

// Schema defines the schema for the BindingPolicy resource
func (r *BindingPolicyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a KubeStellar BindingPolicy. BindingPolicies define which workloads " +
			"from WDS should be deployed to which WECs based on selectors.",
		MarkdownDescription: `
# kubestellar_binding_policy Resource

Manages a KubeStellar **BindingPolicy** for declarative workload distribution.

## What is a BindingPolicy?

BindingPolicies are the core mechanism in KubeStellar for declaring which 
workloads should be deployed to which clusters. They use label selectors to:

1. **Select Workloads**: Match Kubernetes resources in WDS by labels, names, or resource types
2. **Select Clusters**: Match WECs by labels (env, region, etc.)
3. **Define Distribution**: Control how workloads are distributed

## Key Features

- Label-based cluster selection
- Workload selection by resource type, namespace, and labels
- Status collection from WECs
- Integration with GitOps workflows

## Example Usage

### Basic Policy

` + "```hcl" + `
resource "kubestellar_binding_policy" "prod" {
  name      = "prod-policy"
  namespace = "default"

  cluster_selector = {
    match_labels = {
      env = "production"
    }
  }

  downsync {
    api_group = "apps"
    resources = ["deployments"]
    namespaces = ["default"]
    
    object_selectors {
      match_labels = {
        app = "demo"
      }
    }
  }
}
` + "```" + `

### Multi-Region Policy

` + "```hcl" + `
resource "kubestellar_binding_policy" "multi_region" {
  name      = "multi-region-app"
  namespace = "default"

  cluster_selector = {
    match_labels = {
      tier = "frontend"
    }
    match_expressions = [
      {
        key      = "region"
        operator = "In"
        values   = ["us-east-1", "us-west-2", "eu-west-1"]
      }
    ]
  }

  downsync {
    api_group  = "apps"
    resources  = ["deployments", "replicasets"]
    namespaces = ["frontend"]
  }

  downsync {
    api_group  = ""
    resources  = ["services", "configmaps", "secrets"]
    namespaces = ["frontend"]
  }
}
` + "```" + `
`,
		Attributes: map[string]schema.Attribute{
			// Identity fields
			"id": schema.StringAttribute{
				Description: "Unique identifier for the BindingPolicy resource.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the BindingPolicy. Must be unique within the namespace.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 253),
				},
			},
			"namespace": schema.StringAttribute{
				Description: "Namespace where the BindingPolicy is created. Defaults to 'default'.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("default"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"labels": schema.MapAttribute{
				Description: "Labels to apply to the BindingPolicy resource.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"annotations": schema.MapAttribute{
				Description: "Annotations to apply to the BindingPolicy resource.",
				Optional:    true,
				ElementType: types.StringType,
			},

			// Scheduling options
			"want_singleton_reported_state": schema.BoolAttribute{
				Description: "When true, only report state from one cluster. " +
					"Useful for singleton deployments.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},

			// Status fields (computed)
			"status": schema.StringAttribute{
				Description: "Current status of the BindingPolicy.",
				Computed:    true,
			},
			"matched_clusters": schema.ListAttribute{
				Description: "List of cluster names that match the cluster selector.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"matched_workloads": schema.Int64Attribute{
				Description: "Number of workloads that match the downsync selectors.",
				Computed:    true,
			},
			"last_applied_time": schema.StringAttribute{
				Description: "Last time the BindingPolicy was applied to clusters.",
				Computed:    true,
			},
		},
		Blocks: map[string]schema.Block{
			// Cluster selector block
			"cluster_selector": schema.SingleNestedBlock{
				Description: "Selector for target clusters. Matches WEC labels.",
				Attributes: map[string]schema.Attribute{
					"match_labels": schema.MapAttribute{
						Description: "Labels that must match exactly on target clusters.",
						Optional:    true,
						ElementType: types.StringType,
					},
				},
				Blocks: map[string]schema.Block{
					"match_expressions": schema.ListNestedBlock{
						Description: "Label selector requirements with operators.",
						NestedObject: schema.NestedBlockObject{
							Attributes: map[string]schema.Attribute{
								"key": schema.StringAttribute{
									Description: "Label key to match.",
									Required:    true,
								},
								"operator": schema.StringAttribute{
									Description: "Operator for matching. One of: In, NotIn, Exists, DoesNotExist.",
									Required:    true,
									Validators: []validator.String{
										stringvalidator.OneOf("In", "NotIn", "Exists", "DoesNotExist"),
									},
								},
								"values": schema.ListAttribute{
									Description: "Values for In/NotIn operators.",
									Optional:    true,
									ElementType: types.StringType,
								},
							},
						},
					},
				},
			},

			// Downsync blocks (what to deploy)
			"downsync": schema.ListNestedBlock{
				Description: "Workload selectors defining what resources to deploy to matched clusters.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"api_group": schema.StringAttribute{
							Description: "API group of resources to select (e.g., 'apps', '' for core).",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString(""),
						},
						"resources": schema.ListAttribute{
							Description: "Resource types to select (e.g., 'deployments', 'services').",
							Required:    true,
							ElementType: types.StringType,
						},
						"namespaces": schema.ListAttribute{
							Description: "Namespaces to select resources from. Empty means all namespaces.",
							Optional:    true,
							ElementType: types.StringType,
						},
						"namespace_scoped": schema.BoolAttribute{
							Description: "Whether to match namespace-scoped resources. Defaults to true.",
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(true),
						},
						"object_names": schema.ListAttribute{
							Description: "Specific object names to select.",
							Optional:    true,
							ElementType: types.StringType,
						},
						"status_collection": schema.StringAttribute{
							Description: "How to collect status from WECs. Options: 'None', 'Full'.",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString("None"),
							Validators: []validator.String{
								stringvalidator.OneOf("None", "Full"),
							},
						},
					},
					Blocks: map[string]schema.Block{
						"object_selectors": schema.ListNestedBlock{
							Description: "Label selectors for matching specific objects.",
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"match_labels": schema.MapAttribute{
										Description: "Labels that must match exactly.",
										Optional:    true,
										ElementType: types.StringType,
									},
								},
								Blocks: map[string]schema.Block{
									"match_expressions": schema.ListNestedBlock{
										Description: "Label selector requirements.",
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"key": schema.StringAttribute{
													Description: "Label key to match.",
													Required:    true,
												},
												"operator": schema.StringAttribute{
													Description: "Operator for matching.",
													Required:    true,
												},
												"values": schema.ListAttribute{
													Description: "Values for In/NotIn operators.",
													Optional:    true,
													ElementType: types.StringType,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// ValidateConfig validates the resource configuration
func (r *BindingPolicyResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data BindingPolicyResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate that at least one downsync is defined
	if len(data.Downsync) == 0 {
		resp.Diagnostics.AddAttributeError(
			path.Root("downsync"),
			"Missing required block",
			"At least one 'downsync' block must be defined to specify workloads to deploy.",
		)
	}

	// Validate cluster_selector is defined
	if data.ClusterSelector == nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("cluster_selector"),
			"Missing required block",
			"A 'cluster_selector' block must be defined to specify target clusters.",
		)
	}
}

// Configure sets up the resource with provider data
func (r *BindingPolicyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create creates a new BindingPolicy resource
func (r *BindingPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data BindingPolicyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating BindingPolicy", map[string]interface{}{
		"name":      data.Name.ValueString(),
		"namespace": data.Namespace.ValueString(),
	})

	// Build the BindingPolicy custom resource
	bp := r.buildBindingPolicyObject(ctx, data)

	// Create the resource
	gvr := k8sschema.GroupVersionResource{
		Group:    "control.kubestellar.io",
		Version:  "v1alpha1",
		Resource: "bindingpolicies",
	}

	created, err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Create(ctx, bp, metav1.CreateOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to create BindingPolicy",
			fmt.Sprintf("Could not create BindingPolicy %s: %s", data.Name.ValueString(), err),
		)
		return
	}

	// Set computed values
	data.ID = types.StringValue(fmt.Sprintf("%s/%s", data.Namespace.ValueString(), data.Name.ValueString()))
	data.Status = types.StringValue("Pending")
	data.MatchedClusters = types.ListValueMust(types.StringType, []attr.Value{})
	data.MatchedWorkloads = types.Int64Value(0)
	data.LastAppliedTime = types.StringNull()

	// Extract status if available
	r.updateStatusFromObject(ctx, created.Object, &data)

	tflog.Info(ctx, "Created BindingPolicy", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read refreshes the Terraform state with the latest data
func (r *BindingPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data BindingPolicyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading BindingPolicy", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	gvr := k8sschema.GroupVersionResource{
		Group:    "control.kubestellar.io",
		Version:  "v1alpha1",
		Resource: "bindingpolicies",
	}

	bp, err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Get(ctx, data.Name.ValueString(), metav1.GetOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to read BindingPolicy",
			fmt.Sprintf("Could not read BindingPolicy %s: %s", data.Name.ValueString(), err),
		)
		return
	}

	// Update data from the fetched resource
	r.updateDataFromObject(ctx, bp.Object, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update updates the BindingPolicy resource
func (r *BindingPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data BindingPolicyResourceModel
	var state BindingPolicyResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating BindingPolicy", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	gvr := k8sschema.GroupVersionResource{
		Group:    "control.kubestellar.io",
		Version:  "v1alpha1",
		Resource: "bindingpolicies",
	}

	// Get current resource
	current, err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Get(ctx, data.Name.ValueString(), metav1.GetOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to get BindingPolicy for update",
			fmt.Sprintf("Could not get BindingPolicy %s: %s", data.Name.ValueString(), err),
		)
		return
	}

	// Build updated spec
	updated := r.buildBindingPolicyObject(ctx, data)

	// Preserve resource version
	current.Object["spec"] = updated.Object["spec"]
	current.Object["metadata"].(map[string]interface{})["labels"] = updated.Object["metadata"].(map[string]interface{})["labels"]
	current.Object["metadata"].(map[string]interface{})["annotations"] = updated.Object["metadata"].(map[string]interface{})["annotations"]

	// Update the resource
	result, err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Update(ctx, current, metav1.UpdateOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to update BindingPolicy",
			fmt.Sprintf("Could not update BindingPolicy %s: %s", data.Name.ValueString(), err),
		)
		return
	}

	// Update status from response
	r.updateStatusFromObject(ctx, result.Object, &data)
	data.ID = state.ID

	tflog.Info(ctx, "Updated BindingPolicy", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete deletes the BindingPolicy resource
func (r *BindingPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data BindingPolicyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting BindingPolicy", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	gvr := k8sschema.GroupVersionResource{
		Group:    "control.kubestellar.io",
		Version:  "v1alpha1",
		Resource: "bindingpolicies",
	}

	err := r.providerData.DynamicClient.Resource(gvr).
		Namespace(data.Namespace.ValueString()).
		Delete(ctx, data.Name.ValueString(), metav1.DeleteOptions{})
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to delete BindingPolicy",
			fmt.Sprintf("Could not delete BindingPolicy %s: %s", data.Name.ValueString(), err),
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

	tflog.Info(ctx, "Deleted BindingPolicy", map[string]interface{}{
		"id": data.ID.ValueString(),
	})
}

// ImportState imports an existing BindingPolicy resource
func (r *BindingPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Parse the import ID which is in the format: namespace/name
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected import ID in format 'namespace/name', got: %s", req.ID),
		)
		return
	}

	namespace := parts[0]
	name := parts[1]

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("namespace"), namespace)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}

// Helper functions

func (r *BindingPolicyResource) buildBindingPolicyObject(ctx context.Context, data BindingPolicyResourceModel) *unstructured.Unstructured {
	bp := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "control.kubestellar.io/v1alpha1",
			"kind":       "BindingPolicy",
			"metadata": map[string]interface{}{
				"name":      data.Name.ValueString(),
				"namespace": data.Namespace.ValueString(),
			},
			"spec": map[string]interface{}{},
		},
	}

	metadata := bp.Object["metadata"].(map[string]interface{})
	spec := bp.Object["spec"].(map[string]interface{})

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

	// Build cluster selectors
	if data.ClusterSelector != nil {
		clusterSelector := map[string]interface{}{}

		if !data.ClusterSelector.MatchLabels.IsNull() {
			matchLabels := make(map[string]interface{})
			for k, v := range data.ClusterSelector.MatchLabels.Elements() {
				matchLabels[k] = v.(types.String).ValueString()
			}
			clusterSelector["matchLabels"] = matchLabels
		}

		if len(data.ClusterSelector.MatchExpressions) > 0 {
			var expressions []interface{}
			for _, expr := range data.ClusterSelector.MatchExpressions {
				e := map[string]interface{}{
					"key":      expr.Key.ValueString(),
					"operator": expr.Operator.ValueString(),
				}
				if !expr.Values.IsNull() {
					var values []string
					for _, v := range expr.Values.Elements() {
						values = append(values, v.(types.String).ValueString())
					}
					e["values"] = values
				}
				expressions = append(expressions, e)
			}
			clusterSelector["matchExpressions"] = expressions
		}

		spec["clusterSelectors"] = []interface{}{clusterSelector}
	}

	// Build downsync
	var downsyncs []interface{}
	for _, ds := range data.Downsync {
		downsync := map[string]interface{}{
			"apiGroup":        ds.APIGroup.ValueString(),
			"namespaceScoped": ds.NamespaceScoped.ValueBool(),
			"statusCollection": map[string]interface{}{
				"statusCollectionMode": ds.StatusCollection.ValueString(),
			},
		}

		// Resources
		var resources []string
		for _, r := range ds.Resources.Elements() {
			resources = append(resources, r.(types.String).ValueString())
		}
		downsync["resources"] = resources

		// Namespaces
		if !ds.Namespaces.IsNull() {
			var namespaces []string
			for _, ns := range ds.Namespaces.Elements() {
				namespaces = append(namespaces, ns.(types.String).ValueString())
			}
			downsync["namespaces"] = namespaces
		}

		// Object names
		if !ds.ObjectNames.IsNull() {
			var names []string
			for _, n := range ds.ObjectNames.Elements() {
				names = append(names, n.(types.String).ValueString())
			}
			downsync["objectNames"] = names
		}

		// Object selectors
		if !ds.ObjectSelectors.IsNull() {
			var selectors []interface{}
			for _, os := range ds.ObjectSelectors.Elements() {
				osObj := os.(types.Object)
				selector := map[string]interface{}{}

				// Extract match_labels from the object
				attrs := osObj.Attributes()
				if matchLabelsAttr, ok := attrs["match_labels"]; ok {
					if !matchLabelsAttr.IsNull() {
						matchLabelsMap := matchLabelsAttr.(types.Map)
						matchLabels := make(map[string]interface{})
						for k, v := range matchLabelsMap.Elements() {
							matchLabels[k] = v.(types.String).ValueString()
						}
						selector["matchLabels"] = matchLabels
					}
				}

				selectors = append(selectors, selector)
			}
			downsync["objectSelectors"] = selectors
		}

		downsyncs = append(downsyncs, downsync)
	}
	spec["downsync"] = downsyncs

	// Singleton reported state
	spec["wantSingletonReportedState"] = data.WantSingletonReportedState.ValueBool()

	return bp
}

func (r *BindingPolicyResource) updateDataFromObject(ctx context.Context, obj map[string]interface{}, data *BindingPolicyResourceModel) {
	metadata := obj["metadata"].(map[string]interface{})

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

	if annotations, ok := metadata["annotations"].(map[string]interface{}); ok && len(annotations) > 0 {
		annotationMap := make(map[string]string)
		for k, v := range annotations {
			annotationMap[k] = v.(string)
		}
		annotationsValue, _ := types.MapValueFrom(ctx, types.StringType, annotationMap)
		data.Annotations = annotationsValue
	}

	// Read spec fields
	if spec, ok := obj["spec"].(map[string]interface{}); ok {
		// Read clusterSelectors
		if clusterSelectors, ok := spec["clusterSelectors"].([]interface{}); ok && len(clusterSelectors) > 0 {
			// Use the first cluster selector
			if cs, ok := clusterSelectors[0].(map[string]interface{}); ok {
				clusterSelector := &ClusterSelectorModel{}

				if matchLabels, ok := cs["matchLabels"].(map[string]interface{}); ok {
					labelMap := make(map[string]string)
					for k, v := range matchLabels {
						labelMap[k] = v.(string)
					}
					mlValue, _ := types.MapValueFrom(ctx, types.StringType, labelMap)
					clusterSelector.MatchLabels = mlValue
				} else {
					clusterSelector.MatchLabels = types.MapNull(types.StringType)
				}

				if matchExpressions, ok := cs["matchExpressions"].([]interface{}); ok {
					for _, expr := range matchExpressions {
						if exprMap, ok := expr.(map[string]interface{}); ok {
							reqModel := LabelSelectorRequirementModel{
								Key:      types.StringValue(exprMap["key"].(string)),
								Operator: types.StringValue(exprMap["operator"].(string)),
							}
							if values, ok := exprMap["values"].([]interface{}); ok {
								var valList []attr.Value
								for _, v := range values {
									valList = append(valList, types.StringValue(v.(string)))
								}
								reqModel.Values = types.ListValueMust(types.StringType, valList)
							} else {
								reqModel.Values = types.ListNull(types.StringType)
							}
							clusterSelector.MatchExpressions = append(clusterSelector.MatchExpressions, reqModel)
						}
					}
				}

				data.ClusterSelector = clusterSelector
			}
		}

		// Read downsync
		if downsyncs, ok := spec["downsync"].([]interface{}); ok {
			data.Downsync = []DownsyncModel{}
			for _, ds := range downsyncs {
				if dsMap, ok := ds.(map[string]interface{}); ok {
					downsyncModel := DownsyncModel{}

					if apiGroup, ok := dsMap["apiGroup"].(string); ok {
						downsyncModel.APIGroup = types.StringValue(apiGroup)
					} else {
						downsyncModel.APIGroup = types.StringValue("")
					}

					if namespaceScoped, ok := dsMap["namespaceScoped"].(bool); ok {
						downsyncModel.NamespaceScoped = types.BoolValue(namespaceScoped)
					} else {
						downsyncModel.NamespaceScoped = types.BoolValue(true)
					}

					if resources, ok := dsMap["resources"].([]interface{}); ok {
						var resList []attr.Value
						for _, r := range resources {
							resList = append(resList, types.StringValue(r.(string)))
						}
						downsyncModel.Resources = types.ListValueMust(types.StringType, resList)
					} else {
						downsyncModel.Resources = types.ListNull(types.StringType)
					}

					if namespaces, ok := dsMap["namespaces"].([]interface{}); ok {
						var nsList []attr.Value
						for _, ns := range namespaces {
							nsList = append(nsList, types.StringValue(ns.(string)))
						}
						downsyncModel.Namespaces = types.ListValueMust(types.StringType, nsList)
					} else {
						downsyncModel.Namespaces = types.ListNull(types.StringType)
					}

					if objectNames, ok := dsMap["objectNames"].([]interface{}); ok {
						var namesList []attr.Value
						for _, n := range objectNames {
							namesList = append(namesList, types.StringValue(n.(string)))
						}
						downsyncModel.ObjectNames = types.ListValueMust(types.StringType, namesList)
					} else {
						downsyncModel.ObjectNames = types.ListNull(types.StringType)
					}

					// Read status collection
					if statusCol, ok := dsMap["statusCollection"].(map[string]interface{}); ok {
						if mode, ok := statusCol["statusCollectionMode"].(string); ok {
							downsyncModel.StatusCollection = types.StringValue(mode)
						} else {
							downsyncModel.StatusCollection = types.StringValue("None")
						}
					} else {
						downsyncModel.StatusCollection = types.StringValue("None")
					}

					// ObjectSelectors - use the correct type that matches schema
					matchExpressionType := types.ObjectType{
						AttrTypes: map[string]attr.Type{
							"key":      types.StringType,
							"operator": types.StringType,
							"values":   types.ListType{ElemType: types.StringType},
						},
					}
					objectSelectorType := types.ObjectType{
						AttrTypes: map[string]attr.Type{
							"match_labels":      types.MapType{ElemType: types.StringType},
							"match_expressions": types.ListType{ElemType: matchExpressionType},
						},
					}
					downsyncModel.ObjectSelectors = types.ListNull(objectSelectorType)

					data.Downsync = append(data.Downsync, downsyncModel)
				}
			}
		}

		// Read wantSingletonReportedState
		if wantSingleton, ok := spec["wantSingletonReportedState"].(bool); ok {
			data.WantSingletonReportedState = types.BoolValue(wantSingleton)
		} else {
			data.WantSingletonReportedState = types.BoolValue(false)
		}
	}

	r.updateStatusFromObject(ctx, obj, data)
}

func (r *BindingPolicyResource) updateStatusFromObject(ctx context.Context, obj map[string]interface{}, data *BindingPolicyResourceModel) {
	// Set defaults for computed status fields
	if data.Status.IsNull() || data.Status.IsUnknown() {
		data.Status = types.StringValue("Pending")
	}
	if data.MatchedWorkloads.IsNull() || data.MatchedWorkloads.IsUnknown() {
		data.MatchedWorkloads = types.Int64Value(0)
	}
	if data.MatchedClusters.IsNull() || data.MatchedClusters.IsUnknown() {
		data.MatchedClusters = types.ListValueMust(types.StringType, []attr.Value{})
	}
	if data.LastAppliedTime.IsNull() || data.LastAppliedTime.IsUnknown() {
		data.LastAppliedTime = types.StringValue("")
	}

	if status, ok := obj["status"].(map[string]interface{}); ok {
		if phase, ok := status["phase"].(string); ok {
			data.Status = types.StringValue(phase)
		}

		if clusters, ok := status["matchedClusters"].([]interface{}); ok {
			var clusterNames []attr.Value
			for _, c := range clusters {
				clusterNames = append(clusterNames, types.StringValue(c.(string)))
			}
			data.MatchedClusters = types.ListValueMust(types.StringType, clusterNames)
		}

		if count, ok := status["matchedWorkloads"].(int64); ok {
			data.MatchedWorkloads = types.Int64Value(count)
		} else if count, ok := status["matchedWorkloads"].(float64); ok {
			// JSON unmarshaling often produces float64 for numbers
			data.MatchedWorkloads = types.Int64Value(int64(count))
		}

		if lastApplied, ok := status["lastAppliedTime"].(string); ok {
			data.LastAppliedTime = types.StringValue(lastApplied)
		}
	}
}
