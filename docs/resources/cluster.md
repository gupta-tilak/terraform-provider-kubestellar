# kubestellar_cluster Resource

Registers and manages a Workload Execution Cluster (WEC) with KubeStellar.

## Description

WECs are the real Kubernetes clusters where your workloads actually run. This resource registers a cluster with KubeStellar's Inventory Transport Space (ITS), making it available for workload distribution via BindingPolicies.

## Example Usage

### Basic Cluster Registration

```hcl
resource "kubestellar_cluster" "prod" {
  name         = "prod-cluster"
  namespace    = "kubestellar"
  cluster_name = "prod-cluster"

  labels = {
    env = "production"
  }

  kubeconfig = file("~/.kube/prod-cluster.yaml")
}
```

### Fully Configured Cluster

```hcl
resource "kubestellar_cluster" "prod_us_east" {
  name         = "prod-us-east-1"
  namespace    = "kubestellar"
  cluster_name = "prod-us-east-1"

  labels = {
    env      = "production"
    region   = "us-east-1"
    tier     = "frontend"
    provider = "aws"
  }

  annotations = {
    "kubestellar.io/description" = "Production frontend cluster in AWS US East 1"
  }

  description    = "Production cluster in AWS US East 1"
  location       = "us-east-1"
  cloud_provider = "aws"
  environment    = "production"

  kubeconfig = file("kubeconfigs/prod-us-east-1.yaml")
}
```

### Using API Endpoint Instead of Kubeconfig

```hcl
resource "kubestellar_cluster" "external" {
  name         = "external-cluster"
  namespace    = "kubestellar"
  cluster_name = "external-cluster"

  labels = {
    env = "production"
  }

  api_endpoint = "https://external-cluster.example.com:6443"
  ca_data      = file("certs/external-ca.pem")
  token_ref    = "kubestellar/external-cluster-token"
}
```

### Dynamic Cluster Registration

```hcl
variable "clusters" {
  type = map(object({
    region         = string
    environment    = string
    cloud_provider = string
    kubeconfig_path = string
  }))
}

resource "kubestellar_cluster" "dynamic" {
  for_each = var.clusters

  name         = each.key
  namespace    = "kubestellar"
  cluster_name = each.key

  labels = {
    region   = each.value.region
    env      = each.value.environment
    provider = each.value.cloud_provider
  }

  location       = each.value.region
  cloud_provider = each.value.cloud_provider
  environment    = each.value.environment

  kubeconfig = file(each.value.kubeconfig_path)
}
```

## Argument Reference

### Required Arguments

* `name` - (Required, Forces new resource) The name of the cluster registration in KubeStellar.
* `cluster_name` - (Required, Forces new resource) The actual name of the Kubernetes cluster.

### Optional Arguments

* `namespace` - (Optional, Forces new resource) The namespace for registration. Defaults to `"kubestellar"`.
* `labels` - (Optional) Map of labels to apply. These are used by BindingPolicy selectors.
* `annotations` - (Optional) Map of annotations to apply.
* `description` - (Optional) Human-readable description of the cluster.
* `location` - (Optional) Geographic location or region (e.g., `"us-east-1"`).
* `cloud_provider` - (Optional) Cloud provider hosting the cluster. Valid values: `"aws"`, `"gcp"`, `"azure"`, `"on-prem"`, `"kind"`, `"k3d"`, `"minikube"`, `"other"`.
* `environment` - (Optional) Environment type. Valid values: `"production"`, `"staging"`, `"development"`, `"testing"`.

### Connection Configuration (one required)

* `kubeconfig` - (Optional, Sensitive) Full kubeconfig content for connecting to the cluster.
* `api_endpoint` - (Optional) Kubernetes API server endpoint URL.
* `ca_data` - (Optional, Sensitive) PEM-encoded CA certificate for TLS verification.
* `token_ref` - (Optional) Reference to a Secret containing credentials. Format: `"namespace/secret-name"`.

## Attribute Reference

In addition to all arguments above, the following attributes are exported:

* `id` - The resource identifier in format `namespace/name`.
* `status` - Current status of the cluster registration (e.g., `"Ready"`, `"NotReady"`, `"Unknown"`).
* `ready` - Boolean indicating if the cluster is connected and ready.
* `kubernetes_version` - Detected Kubernetes version of the cluster.
* `node_count` - Number of nodes in the cluster.
* `last_sync_time` - Timestamp of the last successful sync.

## Import

Clusters can be imported using the format `namespace/name`:

```bash
terraform import kubestellar_cluster.example kubestellar/my-cluster
```

## Notes

### Immutable Fields

The following fields cannot be changed after creation:
* `name`
* `namespace`
* `cluster_name`

Changing these will force recreation of the resource.

### Mutable Fields

The following fields can be updated in place:
* `labels`
* `annotations`
* `description`
* `location`
* `cloud_provider`
* `environment`
* `kubeconfig`

### Labels for BindingPolicy Selection

Labels are the primary mechanism for BindingPolicy to select target clusters. Common label patterns:

```hcl
labels = {
  # Environment
  env = "production"  # or staging, development
  
  # Geography
  region    = "us-east-1"
  zone      = "us-east-1a"
  country   = "us"
  continent = "north-america"
  
  # Infrastructure
  provider = "aws"  # or gcp, azure, on-prem
  tier     = "frontend"  # or backend, database
  
  # Organization
  team    = "platform"
  project = "my-app"
  
  # Capability
  gpu     = "true"
  storage = "ssd"
}
```

### Kubeconfig Secret

When you provide `kubeconfig`, the provider automatically creates a Secret to store it securely. The secret is named `{cluster-name}-kubeconfig` in the same namespace.

### Connection Validation

The provider validates connectivity to the cluster during creation and updates the status fields. If a cluster becomes unreachable, the `ready` attribute will be `false`.
