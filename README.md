# KubeStellar Terraform Provider

[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org/)
[![Terraform Version](https://img.shields.io/badge/Terraform-1.0+-purple.svg)](https://www.terraform.io/)
[![License](https://img.shields.io/badge/License-Apache%202.0-green.svg)](LICENSE)

The KubeStellar Terraform Provider enables Infrastructure-as-Code (IaC) control of [KubeStellar](https://kubestellar.io/), a multi-cluster Kubernetes orchestration system.

## Overview

This provider allows you to:

- **Provision KubeStellar Control Plane**: Manage Workload Definition Space (WDS) and Inventory Transport Space (ITS)
- **Register Clusters**: Register Workload Execution Clusters (WECs) with KubeStellar
- **Define BindingPolicies**: Create declarative policies for workload distribution across clusters
- **Integrate with GitOps**: Combine Terraform infrastructure management with GitOps workflows

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Terraform Provider                           │
├─────────────────────────────────────────────────────────────────────┤
│  kubestellar_wds    │  kubestellar_its   │  kubestellar_cluster    │
│  kubestellar_binding_policy              │  Data Sources           │
└─────────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                     KubeStellar Architecture                         │
├──────────────────┬─────────────────────┬────────────────────────────┤
│       WDS        │         ITS         │           WECs             │
│  (Workloads &    │  (Cluster Inventory │   (Workload Execution      │
│   Policies)      │   & Transport)      │    Clusters)               │
└──────────────────┴─────────────────────┴────────────────────────────┘
```

## Installation

### Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.21 (for building from source)
- A Kubernetes cluster with KubeStellar installed

### Using the Provider

```hcl
terraform {
  required_providers {
    kubestellar = {
      source  = "kubestellar/kubestellar"
      version = "~> 0.1.0"
    }
  }
}

provider "kubestellar" {
  kubeconfig = "~/.kube/config"
}
```

### Building from Source

```bash
git clone https://github.com/kubestellar/terraform-provider-kubestellar.git
cd terraform-provider-kubestellar

# Build and install
make install
```

## Quick Start

### 1. Configure the Provider

```hcl
provider "kubestellar" {
  kubeconfig = "~/.kube/config"
  context    = "kubestellar-wds"
}
```

### 2. Register a Cluster

```hcl
resource "kubestellar_cluster" "prod" {
  name         = "prod-cluster"
  namespace    = "kubestellar"
  cluster_name = "prod-cluster"

  labels = {
    env      = "production"
    region   = "us-east-1"
    tier     = "frontend"
  }

  kubeconfig = file("~/.kube/prod-cluster.yaml")
}
```

### 3. Create a BindingPolicy

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

### 4. Apply the Configuration

```bash
terraform init
terraform plan
terraform apply
```

## Provider Configuration

| Attribute | Type | Description |
|-----------|------|-------------|
| `kubeconfig` | String | Path to kubeconfig file. Defaults to `~/.kube/config` |
| `kubeconfig_data` | String | Inline kubeconfig content (sensitive) |
| `context` | String | Kubernetes context to use |
| `host` | String | Kubernetes API server URL |
| `token` | String | Bearer token for authentication (sensitive) |
| `in_cluster` | Bool | Use in-cluster configuration |
| `wds_context` | String | Context for WDS operations |
| `its_context` | String | Context for ITS operations |
| `validate_crds` | Bool | Validate KubeStellar CRDs exist (default: true) |

## Resources

### kubestellar_wds

Manages a Workload Definition Space.

```hcl
resource "kubestellar_wds" "main" {
  name       = "main-wds"
  namespace  = "kubestellar"
  space_type = "vcluster"

  labels = {
    environment = "production"
  }
}
```

### kubestellar_its

Manages an Inventory and Transport Space.

```hcl
resource "kubestellar_its" "main" {
  name          = "main-its"
  namespace     = "kubestellar"
  space_type    = "vcluster"
  sync_interval = "30s"
}
```

### kubestellar_cluster

Registers a Workload Execution Cluster.

```hcl
resource "kubestellar_cluster" "wec" {
  name         = "wec-1"
  namespace    = "kubestellar"
  cluster_name = "wec-1"

  labels = {
    env      = "production"
    region   = "us-east-1"
    tier     = "frontend"
  }

  location       = "us-east-1"
  cloud_provider = "aws"
  environment    = "production"

  kubeconfig = file("path/to/kubeconfig.yaml")
}
```

### kubestellar_binding_policy

Defines workload distribution policies.

```hcl
resource "kubestellar_binding_policy" "app" {
  name      = "app-policy"
  namespace = "default"

  cluster_selector {
    match_labels = {
      env = "production"
    }
    match_expressions {
      key      = "region"
      operator = "In"
      values   = ["us-east-1", "us-west-2"]
    }
  }

  downsync {
    api_group  = "apps"
    resources  = ["deployments", "replicasets"]
    namespaces = ["app-namespace"]

    object_selectors {
      match_labels = {
        tier = "frontend"
      }
    }

    status_collection = "Full"
  }

  downsync {
    api_group  = ""
    resources  = ["services", "configmaps", "secrets"]
    namespaces = ["app-namespace"]
  }

  want_singleton_reported_state = false
}
```

## Data Sources

### kubestellar_clusters

Lists registered clusters.

```hcl
data "kubestellar_clusters" "prod" {
  namespace = "kubestellar"
  labels = {
    env = "production"
  }
}

output "prod_clusters" {
  value = [for c in data.kubestellar_clusters.prod.clusters : c.name]
}
```

### kubestellar_binding_policies

Lists binding policies.

```hcl
data "kubestellar_binding_policies" "all" {
  namespace = "default"
}
```

## Examples

See the [examples](./examples/) directory for complete examples:

- [Basic](./examples/basic/) - Simple BindingPolicy example
- [Complete](./examples/complete/) - Full infrastructure setup
- [GitOps Integration](./examples/gitops-integration/) - Combined Terraform + GitOps
- [End-to-End](./examples/end-to-end/) - Complete workflow demonstration

## GitOps + Terraform Integration

The recommended pattern is:

1. **Terraform** manages infrastructure:
   - Cluster provisioning
   - Cluster registration with KubeStellar
   - BindingPolicy definitions

2. **GitOps** (Argo CD/Flux) manages workloads:
   - Deploys applications to WDS
   - KubeStellar distributes to matched WECs

```hcl
# Terraform registers clusters and creates policies
resource "kubestellar_cluster" "prod" {
  name = "prod-cluster"
  labels = { env = "production" }
  # ...
}

resource "kubestellar_binding_policy" "app" {
  cluster_selector {
    match_labels = { env = "production" }
  }
  downsync {
    resources  = ["deployments"]
    namespaces = ["app"]
  }
}

# Argo CD deploys to WDS, KubeStellar distributes
resource "kubernetes_manifest" "argocd_app" {
  manifest = {
    apiVersion = "argoproj.io/v1alpha1"
    kind       = "Application"
    # Deploy to WDS - KubeStellar handles distribution
  }
}
```

## Testing

```bash
# Run unit tests
make test

# Run acceptance tests (requires Kubernetes cluster)
make testacc

# Run integration tests with kind
./test/integration_test.sh all
```

## Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) for details.

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.
