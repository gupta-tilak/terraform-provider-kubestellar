# kubestellar_clusters Data Source

Lists clusters registered with KubeStellar.

## Description

This data source allows you to retrieve information about clusters registered in KubeStellar's Inventory Transport Space (ITS), with optional filtering by labels.

## Example Usage

### List All Clusters

```hcl
data "kubestellar_clusters" "all" {
  namespace = "kubestellar"
}

output "all_clusters" {
  value = [for c in data.kubestellar_clusters.all.clusters : c.name]
}
```

### Filter by Labels

```hcl
data "kubestellar_clusters" "production" {
  namespace = "kubestellar"
  labels = {
    env = "production"
  }
}

output "production_clusters" {
  value = {
    for c in data.kubestellar_clusters.production.clusters : c.name => {
      ready   = c.ready
      version = c.kubernetes_version
      region  = c.labels["region"]
    }
  }
}
```

### Use in BindingPolicy

```hcl
data "kubestellar_clusters" "frontend" {
  namespace = "kubestellar"
  labels = {
    tier = "frontend"
  }
}

# Validate clusters exist before creating policy
resource "kubestellar_binding_policy" "frontend" {
  count = length(data.kubestellar_clusters.frontend.clusters) > 0 ? 1 : 0

  name = "frontend-policy"
  # ...
}
```

## Argument Reference

* `namespace` - (Optional) Namespace to list clusters from.
* `labels` - (Optional) Map of labels to filter clusters.

## Attribute Reference

* `id` - Data source identifier.
* `clusters` - List of cluster objects with:
  * `name` - Cluster registration name.
  * `namespace` - Cluster namespace.
  * `cluster_name` - Actual cluster name.
  * `labels` - Map of cluster labels.
  * `ready` - Boolean ready state.
  * `kubernetes_version` - Detected K8s version.
