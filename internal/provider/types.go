// Copyright 2024 KubeStellar Authors
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Common metadata model used across resources
type MetadataModel struct {
	Name        types.String `tfsdk:"name"`
	Namespace   types.String `tfsdk:"namespace"`
	Labels      types.Map    `tfsdk:"labels"`
	Annotations types.Map    `tfsdk:"annotations"`
}

// LabelSelectorModel represents a Kubernetes label selector
type LabelSelectorModel struct {
	MatchLabels      types.Map                       `tfsdk:"match_labels"`
	MatchExpressions []LabelSelectorRequirementModel `tfsdk:"match_expressions"`
}

// LabelSelectorRequirementModel represents a label selector requirement
type LabelSelectorRequirementModel struct {
	Key      types.String `tfsdk:"key"`
	Operator types.String `tfsdk:"operator"`
	Values   types.List   `tfsdk:"values"`
}

// ClusterSelectorModel represents selector for target clusters
type ClusterSelectorModel struct {
	MatchLabels      types.Map                       `tfsdk:"match_labels"`
	MatchExpressions []LabelSelectorRequirementModel `tfsdk:"match_expressions"`
}

// WorkloadSelectorModel represents selector for workloads
type WorkloadSelectorModel struct {
	MatchLabels      types.Map                       `tfsdk:"match_labels"`
	MatchExpressions []LabelSelectorRequirementModel `tfsdk:"match_expressions"`
}

// DownsyncModel represents downsync configuration in BindingPolicy
type DownsyncModel struct {
	APIGroup        types.String `tfsdk:"api_group"`
	Resources       types.List   `tfsdk:"resources"`
	Namespaces      types.List   `tfsdk:"namespaces"`
	NamespaceScoped types.Bool   `tfsdk:"namespace_scoped"`
	ObjectNames     types.List   `tfsdk:"object_names"`
	ObjectSelectors types.List   `tfsdk:"object_selectors"`
	StatusCollection types.String `tfsdk:"status_collection"`
}

// ObjectSelectorModel represents an object selector
type ObjectSelectorModel struct {
	MatchLabels      types.Map                       `tfsdk:"match_labels"`
	MatchExpressions []LabelSelectorRequirementModel `tfsdk:"match_expressions"`
}

// ClusterStatusModel represents the status of a managed cluster
type ClusterStatusModel struct {
	Phase           types.String `tfsdk:"phase"`
	Ready           types.Bool   `tfsdk:"ready"`
	KubernetesVersion types.String `tfsdk:"kubernetes_version"`
	LastHeartbeat   types.String `tfsdk:"last_heartbeat"`
	Conditions      types.List   `tfsdk:"conditions"`
}

// ConditionModel represents a status condition
type ConditionModel struct {
	Type               types.String `tfsdk:"type"`
	Status             types.String `tfsdk:"status"`
	Reason             types.String `tfsdk:"reason"`
	Message            types.String `tfsdk:"message"`
	LastTransitionTime types.String `tfsdk:"last_transition_time"`
}

// WDSResourceModel describes the data model for WDS
type WDSResourceModel struct {
	ID          types.String  `tfsdk:"id"`
	Name        types.String  `tfsdk:"name"`
	Namespace   types.String  `tfsdk:"namespace"`
	Labels      types.Map     `tfsdk:"labels"`
	Annotations types.Map     `tfsdk:"annotations"`
	
	// Spec fields
	SpaceType   types.String  `tfsdk:"space_type"`
	APIEndpoint types.String  `tfsdk:"api_endpoint"`
	CAData      types.String  `tfsdk:"ca_data"`
	TokenRef    types.String  `tfsdk:"token_ref"`
	
	// Status (computed)
	Status      types.String  `tfsdk:"status"`
	Ready       types.Bool    `tfsdk:"ready"`
}

// ITSResourceModel describes the data model for ITS
type ITSResourceModel struct {
	ID          types.String  `tfsdk:"id"`
	Name        types.String  `tfsdk:"name"`
	Namespace   types.String  `tfsdk:"namespace"`
	Labels      types.Map     `tfsdk:"labels"`
	Annotations types.Map     `tfsdk:"annotations"`
	
	// Spec fields
	SpaceType   types.String  `tfsdk:"space_type"`
	APIEndpoint types.String  `tfsdk:"api_endpoint"`
	CAData      types.String  `tfsdk:"ca_data"`
	TokenRef    types.String  `tfsdk:"token_ref"`
	
	// Cluster inventory settings
	SyncInterval  types.String `tfsdk:"sync_interval"`
	
	// Status (computed)
	Status      types.String  `tfsdk:"status"`
	Ready       types.Bool    `tfsdk:"ready"`
	ClusterCount types.Int64  `tfsdk:"cluster_count"`
}

// ClusterResourceModel describes the data model for registered clusters
type ClusterResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Namespace   types.String `tfsdk:"namespace"`
	Labels      types.Map    `tfsdk:"labels"`
	Annotations types.Map    `tfsdk:"annotations"`
	
	// Spec fields - immutable
	ClusterName types.String `tfsdk:"cluster_name"`
	
	// Spec fields - mutable
	Description    types.String `tfsdk:"description"`
	
	// Connection configuration
	Kubeconfig     types.String `tfsdk:"kubeconfig"`
	APIEndpoint    types.String `tfsdk:"api_endpoint"`
	CAData         types.String `tfsdk:"ca_data"`
	TokenRef       types.String `tfsdk:"token_ref"`
	
	// Cluster properties
	Location       types.String `tfsdk:"location"`
	CloudProvider  types.String `tfsdk:"cloud_provider"`
	Environment    types.String `tfsdk:"environment"`
	
	// Status (computed)
	Status        types.String `tfsdk:"status"`
	Ready         types.Bool   `tfsdk:"ready"`
	KubernetesVersion types.String `tfsdk:"kubernetes_version"`
	NodeCount     types.Int64  `tfsdk:"node_count"`
	LastSyncTime  types.String `tfsdk:"last_sync_time"`
}

// BindingPolicyResourceModel describes the data model for BindingPolicy
type BindingPolicyResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Namespace   types.String `tfsdk:"namespace"`
	Labels      types.Map    `tfsdk:"labels"`
	Annotations types.Map    `tfsdk:"annotations"`
	
	// Cluster selection - specifies which WECs to target
	ClusterSelector   *ClusterSelectorModel `tfsdk:"cluster_selector"`
	ClusterSelectors  types.List            `tfsdk:"cluster_selectors"`
	
	// Workload selection - specifies what to deploy
	Downsync          []DownsyncModel       `tfsdk:"downsync"`
	
	// Scheduling options
	WantSingletonReportedState types.Bool `tfsdk:"want_singleton_reported_state"`
	
	// Status (computed)
	Status            types.String `tfsdk:"status"`
	MatchedClusters   types.List   `tfsdk:"matched_clusters"`
	MatchedWorkloads  types.Int64  `tfsdk:"matched_workloads"`
	LastAppliedTime   types.String `tfsdk:"last_applied_time"`
}
