# kubestellar_binding_policy Resource

Manages a KubeStellar BindingPolicy for declarative workload distribution across multiple Kubernetes clusters.

## Description

BindingPolicies are the core mechanism in KubeStellar for declaring which workloads should be deployed to which clusters. They use label selectors to match both workloads (in the Workload Definition Space) and target clusters (Workload Execution Clusters).

## Example Usage

### Basic BindingPolicy

```hcl
resource "kubestellar_binding_policy" "prod" {
  name      = "prod-policy"
  namespace = "default"

  cluster_selector {
    match_labels = {
      env = "production"
    }
  }

  downsync {
    api_group  = "apps"
    resources  = ["deployments"]
    namespaces = ["default"]

    object_selectors {
      match_labels = {
        app = "demo"
      }
    }

    status_collection = "Full"
  }
}
```

### Multi-Resource Policy

```hcl
resource "kubestellar_binding_policy" "frontend" {
  name      = "frontend-apps"
  namespace = "default"

  labels = {
    team = "frontend"
    tier = "web"
  }

  cluster_selector {
    match_labels = {
      tier = "frontend"
    }
    match_expressions {
      key      = "region"
      operator = "In"
      values   = ["us-east-1", "us-west-2", "eu-west-1"]
    }
  }

  # Deploy Deployments and ReplicaSets
  downsync {
    api_group  = "apps"
    resources  = ["deployments", "replicasets"]
    namespaces = ["frontend-apps"]

    object_selectors {
      match_labels = {
        tier = "frontend"
      }
    }

    status_collection = "Full"
  }

  # Deploy Services
  downsync {
    api_group  = ""
    resources  = ["services"]
    namespaces = ["frontend-apps"]

    object_selectors {
      match_labels = {
        tier = "frontend"
      }
    }
  }

  # Deploy ConfigMaps and Secrets
  downsync {
    api_group  = ""
    resources  = ["configmaps", "secrets"]
    namespaces = ["frontend-apps"]
  }
}
```

### Singleton Deployment

```hcl
resource "kubestellar_binding_policy" "singleton" {
  name      = "singleton-controller"
  namespace = "default"

  # Only report state from one cluster
  want_singleton_reported_state = true

  cluster_selector {
    match_labels = {
      env = "production"
    }
  }

  downsync {
    api_group    = "apps"
    resources    = ["deployments"]
    namespaces   = ["controllers"]
    object_names = ["leader-controller"]

    status_collection = "Full"
  }
}
```

## Argument Reference

### Required Arguments

* `name` - (Required, Forces new resource) The name of the BindingPolicy. Must be unique within the namespace.
* `cluster_selector` - (Required) Block defining which clusters to target. See [Cluster Selector](#cluster-selector) below.
* `downsync` - (Required) One or more blocks defining what workloads to deploy. See [Downsync](#downsync) below.

### Optional Arguments

* `namespace` - (Optional) The namespace for the BindingPolicy. Defaults to `"default"`.
* `labels` - (Optional) Map of labels to apply to the BindingPolicy.
* `annotations` - (Optional) Map of annotations to apply to the BindingPolicy.
* `want_singleton_reported_state` - (Optional) When `true`, only aggregate status from one cluster. Defaults to `false`.

### Cluster Selector

The `cluster_selector` block supports:

* `match_labels` - (Optional) Map of labels that must match exactly on target clusters.
* `match_expressions` - (Optional) List of label selector requirements. Each block supports:
  * `key` - (Required) The label key to match.
  * `operator` - (Required) Operator for matching: `In`, `NotIn`, `Exists`, `DoesNotExist`.
  * `values` - (Optional) List of values for `In`/`NotIn` operators.

### Downsync

The `downsync` block defines which resources to propagate:

* `api_group` - (Optional) API group of resources. Use `""` for core resources, `"apps"` for Deployments, etc.
* `resources` - (Required) List of resource types to select (e.g., `["deployments", "services"]`).
* `namespaces` - (Optional) List of namespaces to select from. Empty means all namespaces.
* `namespace_scoped` - (Optional) Whether to match namespace-scoped resources. Defaults to `true`.
* `object_names` - (Optional) List of specific object names to select.
* `object_selectors` - (Optional) List of label selectors for objects. Each block supports:
  * `match_labels` - Map of labels to match.
  * `match_expressions` - Label selector requirements.
* `status_collection` - (Optional) How to collect status: `"None"` or `"Full"`. Defaults to `"None"`.

## Attribute Reference

In addition to all arguments above, the following attributes are exported:

* `id` - The resource identifier in format `namespace/name`.
* `status` - Current status of the BindingPolicy (e.g., "Active", "Pending").
* `matched_clusters` - List of cluster names that currently match the cluster selector.
* `matched_workloads` - Number of workloads matched by the downsync selectors.
* `last_applied_time` - Timestamp of when the policy was last applied.

## Import

BindingPolicies can be imported using the format `namespace/name`:

```bash
terraform import kubestellar_binding_policy.example default/my-policy
```

## Notes

* Changes to `name` and `namespace` will force recreation of the resource.
* The policy is applied to the WDS and automatically propagated by KubeStellar.
* Use multiple `downsync` blocks to select different resource types with different selectors.
