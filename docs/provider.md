# KubeStellar Terraform Provider Documentation

## Table of Contents

- [Overview](#overview)
- [Installation](#installation)
- [Provider Configuration](#provider-configuration)
- [Resource Reference](#resource-reference)
- [Data Source Reference](#data-source-reference)
- [Common Patterns](#common-patterns)
- [GitOps Integration](#gitops-integration)
- [Troubleshooting](#troubleshooting)

---

## Overview

The KubeStellar Terraform Provider enables declarative management of KubeStellar resources using HashiCorp Terraform. This allows you to:

- Treat KubeStellar infrastructure as code
- Version control your multi-cluster configuration
- Integrate with existing Terraform workflows
- Combine with GitOps for complete automation

### KubeStellar Concepts

Before using this provider, understand these KubeStellar concepts:

| Concept | Description |
|---------|-------------|
| **WDS** (Workload Definition Space) | Kubernetes API where you define workloads and policies |
| **ITS** (Inventory Transport Space) | Registry of all managed clusters |
| **WEC** (Workload Execution Cluster) | Clusters where workloads actually run |
| **BindingPolicy** | CRD that binds workloads to target clusters |

---

## Installation

### From Terraform Registry

```hcl
terraform {
  required_providers {
    kubestellar = {
      source  = "kubestellar/kubestellar"
      version = "~> 0.1.0"
    }
  }
}
```

### From Source

```bash
# Clone the repository
git clone https://github.com/kubestellar/terraform-provider-kubestellar.git
cd terraform-provider-kubestellar

# Build and install
make install

# Verify installation
ls ~/.terraform.d/plugins/registry.terraform.io/kubestellar/kubestellar/
```

---

## Provider Configuration

### Basic Configuration

```hcl
provider "kubestellar" {
  kubeconfig = "~/.kube/config"
}
```

### All Configuration Options

```hcl
provider "kubestellar" {
  # Kubeconfig file path
  kubeconfig = "~/.kube/config"
  
  # Or provide kubeconfig content directly
  kubeconfig_data = var.kubeconfig_content
  
  # Specific context to use
  context = "kubestellar-wds"
  
  # Direct API access (alternative to kubeconfig)
  host  = "https://api.example.com:6443"
  token = var.k8s_token
  
  # TLS configuration
  cluster_ca_certificate = file("ca.crt")
  client_certificate     = file("client.crt")
  client_key             = file("client.key")
  insecure               = false
  
  # In-cluster configuration (for running in pods)
  in_cluster = false
  
  # Separate contexts for WDS and ITS
  wds_context = "wds-cluster"
  its_context = "its-cluster"
  
  # CRD validation
  validate_crds = true
}
```

### Environment Variables

The provider supports these environment variables:

| Variable | Description |
|----------|-------------|
| `KUBECONFIG` | Path to kubeconfig file |
| `KUBE_CONFIG_PATH` | Alternative kubeconfig path |

---

## Resource Reference

### kubestellar_wds

Manages a Workload Definition Space.

#### Schema

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Name of the WDS |
| `namespace` | string | No | Namespace (default: "kubestellar") |
| `labels` | map(string) | No | Resource labels |
| `annotations` | map(string) | No | Resource annotations |
| `space_type` | string | No | Type: "vcluster", "kind", "external" |
| `api_endpoint` | string | No | API endpoint (for external) |
| `ca_data` | string | No | CA certificate data |
| `token_ref` | string | No | Token secret reference |

#### Computed Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | string | Resource identifier |
| `status` | string | Current status |
| `ready` | bool | Ready state |

#### Example

```hcl
resource "kubestellar_wds" "production" {
  name      = "prod-wds"
  namespace = "kubestellar"

  labels = {
    environment = "production"
    team        = "platform"
  }

  space_type = "vcluster"
}
```

---

### kubestellar_its

Manages an Inventory and Transport Space.

#### Schema

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Name of the ITS |
| `namespace` | string | No | Namespace (default: "kubestellar") |
| `labels` | map(string) | No | Resource labels |
| `space_type` | string | No | Type: "vcluster", "kind", "external" |
| `sync_interval` | string | No | Sync interval (default: "30s") |

#### Computed Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | string | Resource identifier |
| `status` | string | Current status |
| `ready` | bool | Ready state |
| `cluster_count` | number | Registered cluster count |

#### Example

```hcl
resource "kubestellar_its" "main" {
  name          = "main-its"
  namespace     = "kubestellar"
  space_type    = "vcluster"
  sync_interval = "1m"
}
```

---

### kubestellar_cluster

Registers a Workload Execution Cluster.

#### Schema

| Attribute | Type | Required | Immutable | Description |
|-----------|------|----------|-----------|-------------|
| `name` | string | Yes | Yes | Registration name |
| `cluster_name` | string | Yes | Yes | Actual cluster name |
| `namespace` | string | No | Yes | Namespace |
| `labels` | map(string) | No | No | Cluster labels |
| `description` | string | No | No | Description |
| `kubeconfig` | string | No | No | Kubeconfig content |
| `api_endpoint` | string | No | No | API endpoint |
| `location` | string | No | No | Geographic location |
| `cloud_provider` | string | No | No | Cloud provider |
| `environment` | string | No | No | Environment type |

#### Computed Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | string | Resource identifier |
| `status` | string | Connection status |
| `ready` | bool | Ready state |
| `kubernetes_version` | string | Detected K8s version |
| `node_count` | number | Node count |
| `last_sync_time` | string | Last sync timestamp |

#### Example

```hcl
resource "kubestellar_cluster" "us_east" {
  name         = "prod-us-east-1"
  cluster_name = "prod-us-east-1"
  namespace    = "kubestellar"

  labels = {
    env      = "production"
    region   = "us-east-1"
    tier     = "frontend"
    provider = "aws"
  }

  description    = "Production frontend cluster"
  location       = "us-east-1"
  cloud_provider = "aws"
  environment    = "production"

  kubeconfig = file("kubeconfigs/prod-us-east-1.yaml")
}
```

---

### kubestellar_binding_policy

Defines workload-to-cluster binding rules.

#### Schema

| Attribute | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Policy name |
| `namespace` | string | No | Namespace (default: "default") |
| `labels` | map(string) | No | Resource labels |
| `want_singleton_reported_state` | bool | No | Singleton reporting |

#### Blocks

##### cluster_selector (required)

```hcl
cluster_selector {
  match_labels = {
    env = "production"
  }
  
  match_expressions {
    key      = "region"
    operator = "In"       # In, NotIn, Exists, DoesNotExist
    values   = ["us-east-1", "us-west-2"]
  }
}
```

##### downsync (required, multiple allowed)

```hcl
downsync {
  api_group        = "apps"           # "" for core resources
  resources        = ["deployments"]
  namespaces       = ["app-ns"]
  namespace_scoped = true
  object_names     = ["specific-deployment"]
  
  object_selectors {
    match_labels = {
      app = "demo"
    }
  }
  
  status_collection = "Full"  # None, Full
}
```

#### Computed Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `matched_clusters` | list(string) | Matched cluster names |
| `matched_workloads` | number | Matched workload count |
| `last_applied_time` | string | Last application time |

#### Example

```hcl
resource "kubestellar_binding_policy" "frontend" {
  name      = "frontend-policy"
  namespace = "default"

  labels = {
    tier = "frontend"
  }

  cluster_selector {
    match_labels = {
      env  = "production"
      tier = "frontend"
    }
  }

  # Deploy Deployments
  downsync {
    api_group  = "apps"
    resources  = ["deployments", "replicasets"]
    namespaces = ["frontend"]

    object_selectors {
      match_labels = {
        tier = "frontend"
      }
    }

    status_collection = "Full"
  }

  # Deploy Services and ConfigMaps
  downsync {
    api_group  = ""
    resources  = ["services", "configmaps", "secrets"]
    namespaces = ["frontend"]
  }
}
```

---

## Data Source Reference

### kubestellar_clusters

Lists registered clusters with optional filtering.

```hcl
data "kubestellar_clusters" "all" {
  namespace = "kubestellar"
}

data "kubestellar_clusters" "prod" {
  namespace = "kubestellar"
  labels = {
    env = "production"
  }
}

output "production_clusters" {
  value = [for c in data.kubestellar_clusters.prod.clusters : {
    name    = c.name
    ready   = c.ready
    version = c.kubernetes_version
  }]
}
```

### kubestellar_binding_policies

Lists binding policies.

```hcl
data "kubestellar_binding_policies" "all" {
  namespace = "default"
}
```

---

## Common Patterns

### Multi-Region Deployment

```hcl
locals {
  regions = {
    "us-east-1" = { env = "production" }
    "us-west-2" = { env = "production" }
    "eu-west-1" = { env = "production" }
  }
}

resource "kubestellar_cluster" "regional" {
  for_each = local.regions

  name         = "cluster-${each.key}"
  cluster_name = "cluster-${each.key}"

  labels = merge(each.value, {
    region = each.key
  })

  kubeconfig = file("kubeconfigs/${each.key}.yaml")
}

resource "kubestellar_binding_policy" "global" {
  name = "global-app"

  cluster_selector {
    match_labels = {
      env = "production"
    }
  }

  downsync {
    api_group  = "apps"
    resources  = ["deployments"]
    namespaces = ["global-apps"]
  }
}
```

### Tiered Deployment

```hcl
# Canary: deploy to one cluster
resource "kubestellar_binding_policy" "canary" {
  name = "canary"

  want_singleton_reported_state = true

  cluster_selector {
    match_labels = {
      env  = "production"
      tier = "canary"
    }
  }

  downsync {
    api_group  = "apps"
    resources  = ["deployments"]
    namespaces = ["app"]

    object_selectors {
      match_labels = {
        release = "canary"
      }
    }
  }
}

# Stable: deploy to all production clusters
resource "kubestellar_binding_policy" "stable" {
  name = "stable"

  cluster_selector {
    match_labels = {
      env = "production"
    }
  }

  downsync {
    api_group  = "apps"
    resources  = ["deployments"]
    namespaces = ["app"]

    object_selectors {
      match_labels = {
        release = "stable"
      }
    }
  }
}
```

---

## GitOps Integration

### With Argo CD

```hcl
# Terraform manages infrastructure
resource "kubestellar_binding_policy" "apps" {
  # ... policy definition
}

# Argo CD Application deploying to WDS
resource "kubernetes_manifest" "argocd_app" {
  manifest = {
    apiVersion = "argoproj.io/v1alpha1"
    kind       = "Application"
    metadata = {
      name      = "my-app"
      namespace = "argocd"
    }
    spec = {
      project = "default"
      source = {
        repoURL        = "https://github.com/org/app.git"
        targetRevision = "main"
        path           = "k8s"
      }
      destination = {
        server    = "https://kubernetes.default.svc"
        namespace = "app"
      }
      syncPolicy = {
        automated = {
          prune    = true
          selfHeal = true
        }
      }
    }
  }
}
```

### With Flux

```hcl
# Flux GitRepository
resource "kubernetes_manifest" "flux_gitrepo" {
  manifest = {
    apiVersion = "source.toolkit.fluxcd.io/v1"
    kind       = "GitRepository"
    metadata = {
      name      = "my-app"
      namespace = "flux-system"
    }
    spec = {
      interval = "1m"
      url      = "https://github.com/org/app.git"
      ref = {
        branch = "main"
      }
    }
  }
}

# Flux Kustomization
resource "kubernetes_manifest" "flux_kustomization" {
  manifest = {
    apiVersion = "kustomize.toolkit.fluxcd.io/v1"
    kind       = "Kustomization"
    metadata = {
      name      = "my-app"
      namespace = "flux-system"
    }
    spec = {
      interval = "5m"
      path     = "./k8s"
      prune    = true
      sourceRef = {
        kind = "GitRepository"
        name = "my-app"
      }
    }
  }
}
```

---

## Troubleshooting

### CRD Not Found

```
Error: KubeStellar CRDs not found
```

**Solution**: Install KubeStellar CRDs before using the provider:

```bash
kubectl apply -f https://raw.githubusercontent.com/kubestellar/kubestellar/main/config/crd/bases/
```

Or disable CRD validation:

```hcl
provider "kubestellar" {
  validate_crds = false
}
```

### Authentication Failed

```
Error: Failed to create Kubernetes client
```

**Solution**: Verify kubeconfig:

```bash
kubectl cluster-info --kubeconfig ~/.kube/config
```

### Cluster Not Ready

Check cluster status:

```hcl
output "cluster_status" {
  value = kubestellar_cluster.my_cluster.status
}
```

```bash
kubectl get managedclusters -n kubestellar
```

### Import Existing Resources

```bash
terraform import kubestellar_binding_policy.existing default/existing-policy
terraform import kubestellar_cluster.existing kubestellar/existing-cluster
```
