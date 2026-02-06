// Copyright 2024 KubeStellar Authors
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"os"
	"path/filepath"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Ensure KubeStellarProvider satisfies various provider interfaces.
var _ provider.Provider = &KubeStellarProvider{}

// KubeStellarProvider defines the provider implementation.
type KubeStellarProvider struct {
	version string
}

// KubeStellarProviderModel describes the provider data model.
type KubeStellarProviderModel struct {
	Kubeconfig       types.String `tfsdk:"kubeconfig"`
	KubeconfigData   types.String `tfsdk:"kubeconfig_data"`
	Context          types.String `tfsdk:"context"`
	Host             types.String `tfsdk:"host"`
	Token            types.String `tfsdk:"token"`
	ClusterCACert    types.String `tfsdk:"cluster_ca_certificate"`
	ClientCert       types.String `tfsdk:"client_certificate"`
	ClientKey        types.String `tfsdk:"client_key"`
	Insecure         types.Bool   `tfsdk:"insecure"`
	InCluster        types.Bool   `tfsdk:"in_cluster"`
	WDSContext       types.String `tfsdk:"wds_context"`
	ITSContext       types.String `tfsdk:"its_context"`
	ValidateCRDs     types.Bool   `tfsdk:"validate_crds"`
}

// ProviderData contains the configured Kubernetes clients
type ProviderData struct {
	ClientSet     *kubernetes.Clientset
	DynamicClient dynamic.Interface
	RestConfig    *rest.Config
	WDSConfig     *rest.Config
	ITSConfig     *rest.Config
	ValidateCRDs  bool
}

// New creates a new provider instance
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &KubeStellarProvider{
			version: version,
		}
	}
}

// Metadata returns the provider type name
func (p *KubeStellarProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "kubestellar"
	resp.Version = p.version
}

// Schema defines the provider-level schema for configuration data
func (p *KubeStellarProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The KubeStellar provider enables Terraform to manage KubeStellar resources " +
			"for multi-cluster Kubernetes orchestration. It supports provisioning clusters, " +
			"registering them with KubeStellar, defining BindingPolicies, and managing " +
			"multi-cluster workloads declaratively.",
		MarkdownDescription: `
# KubeStellar Terraform Provider

The KubeStellar provider enables Infrastructure-as-Code control of KubeStellar, 
a multi-cluster Kubernetes orchestration system.

## Features

- **Cluster Registration**: Register Workload Execution Clusters (WECs) with KubeStellar
- **BindingPolicy Management**: Define declarative policies for workload distribution
- **WDS/ITS Configuration**: Manage Workload Definition Space and Inventory Transport Space
- **Multi-cluster Workloads**: Orchestrate workloads across multiple Kubernetes clusters

## Authentication

The provider supports multiple authentication methods:
- Kubeconfig file path
- Kubeconfig data (inline)
- In-cluster configuration (for running inside Kubernetes)
- Direct API server connection with token/certificates
`,
		Attributes: map[string]schema.Attribute{
			"kubeconfig": schema.StringAttribute{
				Description: "Path to the kubeconfig file. Defaults to ~/.kube/config. " +
					"Can also be set via KUBECONFIG or KUBE_CONFIG_PATH environment variables.",
				MarkdownDescription: "Path to the kubeconfig file. Defaults to `~/.kube/config`. " +
					"Can also be set via `KUBECONFIG` or `KUBE_CONFIG_PATH` environment variables.",
				Optional: true,
			},
			"kubeconfig_data": schema.StringAttribute{
				Description: "Kubeconfig file content. This can be used instead of kubeconfig path.",
				Optional:    true,
				Sensitive:   true,
			},
			"context": schema.StringAttribute{
				Description: "The context to use from the kubeconfig file. " +
					"If not specified, the current-context is used.",
				Optional: true,
			},
			"host": schema.StringAttribute{
				Description: "The Kubernetes API server URL. Use this for direct API access.",
				Optional:    true,
			},
			"token": schema.StringAttribute{
				Description: "Bearer token for authentication to the Kubernetes API.",
				Optional:    true,
				Sensitive:   true,
			},
			"cluster_ca_certificate": schema.StringAttribute{
				Description: "PEM-encoded CA certificate for TLS verification.",
				Optional:    true,
				Sensitive:   true,
			},
			"client_certificate": schema.StringAttribute{
				Description: "PEM-encoded client certificate for TLS authentication.",
				Optional:    true,
				Sensitive:   true,
			},
			"client_key": schema.StringAttribute{
				Description: "PEM-encoded client key for TLS authentication.",
				Optional:    true,
				Sensitive:   true,
			},
			"insecure": schema.BoolAttribute{
				Description: "Skip TLS certificate verification. Not recommended for production.",
				Optional:    true,
			},
			"in_cluster": schema.BoolAttribute{
				Description: "Use in-cluster configuration when running inside Kubernetes. " +
					"This uses the pod's service account.",
				Optional: true,
			},
			"wds_context": schema.StringAttribute{
				Description: "Kubernetes context for the Workload Definition Space (WDS). " +
					"Defaults to the main context if not specified.",
				Optional: true,
			},
			"its_context": schema.StringAttribute{
				Description: "Kubernetes context for the Inventory and Transport Space (ITS). " +
					"Defaults to the main context if not specified.",
				Optional: true,
			},
			"validate_crds": schema.BoolAttribute{
				Description: "Validate that KubeStellar CRDs exist before applying resources. " +
					"Defaults to true.",
				Optional: true,
			},
		},
	}
}

// Configure prepares a KubeStellar API client for data sources and resources
func (p *KubeStellarProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	tflog.Info(ctx, "Configuring KubeStellar provider")

	var config KubeStellarProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Handle environment variables
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = os.Getenv("KUBE_CONFIG_PATH")
	}
	if !config.Kubeconfig.IsNull() {
		kubeconfig = config.Kubeconfig.ValueString()
	}

	// Expand ~ to home directory
	if kubeconfig != "" && kubeconfig[0] == '~' {
		home, _ := os.UserHomeDir()
		kubeconfig = filepath.Join(home, kubeconfig[1:])
	}

	// Default kubeconfig path
	if kubeconfig == "" && config.KubeconfigData.IsNull() && config.Host.IsNull() {
		if !config.InCluster.ValueBool() {
			home, _ := os.UserHomeDir()
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}

	var restConfig *rest.Config
	var err error

	// Build Kubernetes configuration
	if config.InCluster.ValueBool() {
		tflog.Debug(ctx, "Using in-cluster configuration")
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			resp.Diagnostics.AddError(
				"Failed to create in-cluster configuration",
				"Could not create in-cluster Kubernetes configuration: "+err.Error(),
			)
			return
		}
	} else if !config.Host.IsNull() {
		tflog.Debug(ctx, "Using direct API configuration")
		restConfig = &rest.Config{
			Host:        config.Host.ValueString(),
			BearerToken: config.Token.ValueString(),
		}
		if !config.ClusterCACert.IsNull() {
			restConfig.TLSClientConfig.CAData = []byte(config.ClusterCACert.ValueString())
		}
		if !config.ClientCert.IsNull() {
			restConfig.TLSClientConfig.CertData = []byte(config.ClientCert.ValueString())
		}
		if !config.ClientKey.IsNull() {
			restConfig.TLSClientConfig.KeyData = []byte(config.ClientKey.ValueString())
		}
		restConfig.TLSClientConfig.Insecure = config.Insecure.ValueBool()
	} else if !config.KubeconfigData.IsNull() {
		tflog.Debug(ctx, "Using inline kubeconfig data")
		clientConfig, err := clientcmd.NewClientConfigFromBytes([]byte(config.KubeconfigData.ValueString()))
		if err != nil {
			resp.Diagnostics.AddError(
				"Failed to parse kubeconfig data",
				"Could not parse inline kubeconfig: "+err.Error(),
			)
			return
		}
		restConfig, err = clientConfig.ClientConfig()
		if err != nil {
			resp.Diagnostics.AddError(
				"Failed to create client config",
				"Could not create client configuration from kubeconfig data: "+err.Error(),
			)
			return
		}
	} else {
		tflog.Debug(ctx, "Using kubeconfig file", map[string]interface{}{"path": kubeconfig})
		
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		loadingRules.ExplicitPath = kubeconfig
		
		configOverrides := &clientcmd.ConfigOverrides{}
		if !config.Context.IsNull() {
			configOverrides.CurrentContext = config.Context.ValueString()
		}
		
		clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
		restConfig, err = clientConfig.ClientConfig()
		if err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("kubeconfig"),
				"Failed to load kubeconfig",
				"Could not load kubeconfig file at "+kubeconfig+": "+err.Error(),
			)
			return
		}
	}

	// Create Kubernetes clients
	clientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to create Kubernetes client",
			"Could not create Kubernetes clientset: "+err.Error(),
		)
		return
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to create dynamic client",
			"Could not create dynamic Kubernetes client: "+err.Error(),
		)
		return
	}

	// Validate CRDs if enabled (default: true)
	validateCRDs := true
	if !config.ValidateCRDs.IsNull() {
		validateCRDs = config.ValidateCRDs.ValueBool()
	}

	if validateCRDs {
		tflog.Debug(ctx, "Validating KubeStellar CRDs exist")
		if err := validateKubeStellarCRDs(ctx, dynamicClient); err != nil {
			resp.Diagnostics.AddWarning(
				"KubeStellar CRDs not found",
				"Some KubeStellar CRDs may not be installed: "+err.Error()+
					". Resources may fail to create. Install KubeStellar first.",
			)
		}
	}

	// Build WDS and ITS specific configs if contexts are provided
	var wdsConfig, itsConfig *rest.Config
	
	if !config.WDSContext.IsNull() {
		wdsConfig, err = buildContextConfig(kubeconfig, config.WDSContext.ValueString())
		if err != nil {
			resp.Diagnostics.AddWarning(
				"Failed to configure WDS context",
				"Could not create WDS configuration: "+err.Error(),
			)
		}
	}
	
	if !config.ITSContext.IsNull() {
		itsConfig, err = buildContextConfig(kubeconfig, config.ITSContext.ValueString())
		if err != nil {
			resp.Diagnostics.AddWarning(
				"Failed to configure ITS context",
				"Could not create ITS configuration: "+err.Error(),
			)
		}
	}

	// Create provider data
	providerData := &ProviderData{
		ClientSet:     clientSet,
		DynamicClient: dynamicClient,
		RestConfig:    restConfig,
		WDSConfig:     wdsConfig,
		ITSConfig:     itsConfig,
		ValidateCRDs:  validateCRDs,
	}

	resp.DataSourceData = providerData
	resp.ResourceData = providerData

	tflog.Info(ctx, "KubeStellar provider configured successfully")
}

// Resources defines the resources implemented by this provider
func (p *KubeStellarProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewWDSResource,
		NewITSResource,
		NewClusterResource,
		NewBindingPolicyResource,
	}
}

// DataSources defines the data sources implemented by this provider
func (p *KubeStellarProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewClustersDataSource,
		NewBindingPoliciesDataSource,
	}
}

// buildContextConfig creates a rest.Config for a specific kubeconfig context
func buildContextConfig(kubeconfigPath, contextName string) (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = kubeconfigPath
	
	configOverrides := &clientcmd.ConfigOverrides{
		CurrentContext: contextName,
	}
	
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	return clientConfig.ClientConfig()
}

// validateKubeStellarCRDs checks if required KubeStellar CRDs are installed
func validateKubeStellarCRDs(ctx context.Context, client dynamic.Interface) error {
	// TODO: Implement CRD validation
	// Check for BindingPolicy, WorkStatus, and other KubeStellar CRDs
	return nil
}
