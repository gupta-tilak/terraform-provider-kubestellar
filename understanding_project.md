# Understanding the KubeStellar Terraform Provider Project

## Table of Contents

1. [What is Terraform?](#1-what-is-terraform)
2. [What is Kubernetes Multi-Cluster?](#2-what-is-kubernetes-multi-cluster)
3. [What is KubeStellar?](#3-what-is-kubestellar)
4. [What This Project Does](#4-what-this-project-does)
5. [Project Structure Explained](#5-project-structure-explained)
6. [Key Resources Explained](#6-key-resources-explained)
7. [Real-World Workflow](#7-real-world-workflow)
8. [Understanding the Test File](#8-understanding-the-test-file)
9. [How to Use This Project](#9-how-to-use-this-project)
10. [GitOps Integration](#10-gitops-integration)
11. [Key Takeaways](#11-key-takeaways)

---

## 1. What is Terraform?

**Terraform** is an Infrastructure-as-Code (IaC) tool that lets you define infrastructure using configuration files instead of manual setup.

```hcl
# Instead of clicking buttons in AWS console, you write:
resource "aws_instance" "web" {
  ami           = "ami-12345"
  instance_type = "t2.micro"
}
```

### Key Concepts

| Concept | Description |
|---------|-------------|
| **Provider** | Plugin that talks to an API (AWS, GCP, Kubernetes, etc.) |
| **Resource** | Something Terraform creates/manages (server, database, etc.) |
| **Data Source** | Read-only query to get existing information |
| **State** | Terraform tracks what it created in a state file |

---

## 2. What is Kubernetes Multi-Cluster?

Normally, you have **one** Kubernetes cluster. But enterprises often need **many clusters**:

```
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│   US-East       │  │   US-West       │  │   EU-West       │
│   Cluster       │  │   Cluster       │  │   Cluster       │
└─────────────────┘  └─────────────────┘  └─────────────────┘
```

### Why Multiple Clusters?

- **Geographic distribution** - Low latency for users worldwide
- **Environment isolation** - Separate dev/staging/prod
- **Compliance** - Data residency laws require regional clusters
- **High availability** - Survive regional outages

### The Problem

How do you deploy the same app to all clusters without manually running `kubectl apply` on each one?

---

## 3. What is KubeStellar?

**KubeStellar** solves multi-cluster workload distribution. It has three core components:

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                     KubeStellar Architecture                     │
├──────────────────┬────────────────────┬─────────────────────────┤
│       WDS        │        ITS         │          WECs           │
│  (Control Plane) │  (Cluster Registry)│  (Actual Clusters)      │
├──────────────────┼────────────────────┼─────────────────────────┤
│ • Define apps    │ • Lists all        │ • Where apps run        │
│ • Define policies│   clusters         │ • Real K8s clusters     │
│ • Central hub    │ • Cluster metadata │ • Can be anywhere       │
└──────────────────┴────────────────────┴─────────────────────────┘
```

### Component Breakdown

| Component | Full Name | Purpose |
|-----------|-----------|---------|
| **WDS** | Workload Definition Space | Where you define what to deploy |
| **ITS** | Inventory Transport Space | Registry of all your clusters |
| **WEC** | Workload Execution Cluster | Actual clusters where apps run |
| **BindingPolicy** | - | Rules for which apps go to which clusters |

### How It Works

```
Step 1: Register clusters with KubeStellar (ITS knows about them)
Step 2: Label clusters (env=production, region=us-east-1)
Step 3: Create BindingPolicy (deploy to clusters with env=production)
Step 4: Deploy app to WDS
Step 5: KubeStellar automatically distributes to matching WECs
```

---

## 4. What This Project Does

This **Terraform Provider** lets you manage KubeStellar using Terraform instead of `kubectl`:

### Without This Provider (Manual)

```bash
# You would run these commands manually:
kubectl apply -f wds.yaml
kubectl apply -f its.yaml
kubectl apply -f cluster-registration.yaml
kubectl apply -f binding-policy.yaml
```

### With This Provider (Automated)

```hcl
# Define everything in code:
resource "kubestellar_wds" "main" {
  name = "main-wds"
}

resource "kubestellar_its" "main" {
  name = "main-its"
}

resource "kubestellar_cluster" "prod" {
  name   = "prod-cluster"
  labels = { env = "production" }
}

resource "kubestellar_binding_policy" "app" {
  cluster_selector {
    match_labels = { env = "production" }
  }
  downsync {
    resources = ["deployments"]
  }
}
```

---

## 5. Project Structure Explained

```
terraform-provider-kubestellar/
├── main.go                    # Entry point - starts the provider
├── go.mod                     # Go dependencies
├── Makefile                   # Build commands
│
├── internal/provider/         # Core provider code
│   ├── provider.go            # Provider configuration
│   ├── resource_wds.go        # WDS resource implementation
│   ├── resource_its.go        # ITS resource implementation
│   ├── resource_cluster.go    # Cluster registration
│   ├── resource_binding_policy.go  # BindingPolicy resource
│   ├── data_sources.go        # Data sources (read-only queries)
│   ├── types.go               # Data structures
│   └── provider_test.go       # Tests
│
├── examples/                  # Usage examples
│   ├── basic/                 # Simple example
│   ├── complete/              # Full infrastructure
│   ├── end-to-end/            # Complete workflow
│   └── gitops-integration/    # With Argo CD/Flux
│
├── docs/                      # Documentation
│   ├── provider.md            # Provider docs
│   ├── resources/             # Resource docs
│   └── data-sources/          # Data source docs
│
└── test/                      # Integration tests
    ├── integration_test.sh    # Test script
    └── crds/                  # Test CRDs
```

---

## 6. Key Resources Explained

### 6.1 `kubestellar_wds` - Workload Definition Space

The central control plane where you define workloads:

```hcl
resource "kubestellar_wds" "main" {
  name       = "main-wds"
  namespace  = "kubestellar"
  space_type = "vcluster"  # vcluster, kind, or external
  
  labels = {
    environment = "production"
  }
}
```

### 6.2 `kubestellar_its` - Inventory Transport Space

Registry that tracks all your clusters:

```hcl
resource "kubestellar_its" "main" {
  name          = "main-its"
  namespace     = "kubestellar"
  sync_interval = "30s"  # How often to sync cluster status
}
```

### 6.3 `kubestellar_cluster` - Cluster Registration

Register an existing Kubernetes cluster with KubeStellar:

```hcl
resource "kubestellar_cluster" "prod_us_east" {
  name         = "prod-us-east-1"
  cluster_name = "prod-us-east-1"
  namespace    = "kubestellar"
  
  # Labels are KEY - used by BindingPolicies to select clusters
  labels = {
    env      = "production"
    region   = "us-east-1"
    tier     = "frontend"
    provider = "aws"
  }
  
  location       = "us-east-1"
  cloud_provider = "aws"
  environment    = "production"
  
  # Credentials to access the cluster
  kubeconfig = file("~/.kube/prod-us-east-1.yaml")
}
```

### 6.4 `kubestellar_binding_policy` - Distribution Rules

**This is the most important resource** - it defines WHAT gets deployed WHERE:

```hcl
resource "kubestellar_binding_policy" "frontend" {
  name      = "frontend-policy"
  namespace = "default"
  
  # WHERE: Select target clusters by labels
  cluster_selector {
    match_labels = {
      env  = "production"
      tier = "frontend"
    }
    
    # Or use expressions for more complex matching
    match_expressions {
      key      = "region"
      operator = "In"           # In, NotIn, Exists, DoesNotExist
      values   = ["us-east-1", "us-west-2"]
    }
  }
  
  # WHAT: Select workloads to distribute
  downsync {
    api_group  = "apps"              # apps, "", networking.k8s.io, etc.
    resources  = ["deployments"]     # Which resource types
    namespaces = ["frontend"]        # From which namespaces
    
    # Optional: Further filter by labels on the workloads
    object_selectors {
      match_labels = {
        tier = "frontend"
      }
    }
    
    status_collection = "Full"  # None or Full - collect status back
  }
  
  # Can have multiple downsync blocks
  downsync {
    api_group  = ""  # Core resources (Services, ConfigMaps)
    resources  = ["services", "configmaps"]
    namespaces = ["frontend"]
  }
}
```

---

## 7. Real-World Workflow

Here's a complete example of how you'd use this in production:

### Step 1: Configure Provider

```hcl
# main.tf
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
  context    = "kubestellar-admin"
}
```

### Step 2: Set Up Control Plane

```hcl
resource "kubestellar_wds" "main" {
  name       = "production-wds"
  namespace  = "kubestellar"
  space_type = "vcluster"
}

resource "kubestellar_its" "main" {
  name          = "production-its"
  namespace     = "kubestellar"
  sync_interval = "30s"
}
```

### Step 3: Register Your Clusters

```hcl
# Register multiple clusters using for_each
locals {
  clusters = {
    "prod-us-east" = {
      region = "us-east-1"
      env    = "production"
    }
    "prod-us-west" = {
      region = "us-west-2"
      env    = "production"
    }
    "staging" = {
      region = "us-central-1"
      env    = "staging"
    }
  }
}

resource "kubestellar_cluster" "clusters" {
  for_each = local.clusters
  
  name         = each.key
  cluster_name = each.key
  
  labels = {
    env    = each.value.env
    region = each.value.region
  }
  
  kubeconfig = file("kubeconfigs/${each.key}.yaml")
  
  depends_on = [kubestellar_its.main]
}
```

### Step 4: Create Distribution Policies

```hcl
# Deploy to all production clusters
resource "kubestellar_binding_policy" "prod_apps" {
  name      = "production-apps"
  namespace = "default"
  
  cluster_selector {
    match_labels = {
      env = "production"
    }
  }
  
  downsync {
    api_group  = "apps"
    resources  = ["deployments", "statefulsets"]
    namespaces = ["app"]
    
    status_collection = "Full"
  }
  
  downsync {
    api_group  = ""
    resources  = ["services", "configmaps", "secrets"]
    namespaces = ["app"]
  }
  
  depends_on = [kubestellar_cluster.clusters]
}

# Deploy to staging only
resource "kubestellar_binding_policy" "staging" {
  name = "staging-all"
  
  cluster_selector {
    match_labels = {
      env = "staging"
    }
  }
  
  downsync {
    api_group  = "apps"
    resources  = ["deployments"]
    namespaces = ["staging"]
  }
}
```

### Step 5: Query Cluster Status

```hcl
# Data sources to read existing information
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

### Step 6: Apply

```bash
terraform init
terraform plan
terraform apply
```

---

## 8. Understanding the Test File

The test file (`internal/provider/provider_test.go`) contains two types of tests:

### Acceptance Tests (Require Real Kubernetes Cluster)

```go
func TestAccResourceBindingPolicy_basic(t *testing.T) {
    resource.Test(t, resource.TestCase{
        PreCheck: func() { testAccPreCheck(t) },  // Verify environment
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: testAccResourceBindingPolicyConfig_basic(),
                Check: resource.ComposeAggregateTestCheckFunc(
                    // Verify resource was created correctly
                    resource.TestCheckResourceAttr("kubestellar_binding_policy.test", "name", "test-policy"),
                ),
            },
            {
                // Test import functionality
                ResourceName:      "kubestellar_binding_policy.test",
                ImportState:       true,
                ImportStateVerify: true,
            },
        },
    })
}
```

### Unit Tests (Don't Need Kubernetes)

```go
func TestProviderSchema(t *testing.T) {
    // Validates the schema is correctly defined
    // These run quickly without external dependencies
}
```

---

## 9. How to Use This Project

### Prerequisites

| Requirement | Version | Purpose |
|-------------|---------|---------|
| Go | 1.21+ | The provider is written in Go |
| Terraform | 1.0+ | To use the provider |
| Kubernetes cluster | Any | With KubeStellar installed |
| kubectl | Latest | For cluster access |

### Building from Source

```bash
# Clone the repo
git clone https://github.com/kubestellar/terraform-provider-kubestellar.git
cd terraform-provider-kubestellar

# Build and install locally
make install

# Run tests
make test

# Run acceptance tests (needs K8s cluster)
make testacc
```

### Quick Test with Kind

```bash
# Create test clusters
make kind-create

# Install KubeStellar CRDs
make install-crds

# Run integration tests
./test/integration_test.sh all

# Cleanup
make kind-delete
```

---

## 10. GitOps Integration

The recommended pattern combines Terraform with GitOps tools:

```
┌──────────────────────────────────────────────────────────────┐
│                    Deployment Flow                            │
├──────────────────────────────────────────────────────────────┤
│                                                               │
│  Terraform                    GitOps (Argo CD/Flux)          │
│  ─────────                    ─────────────────────          │
│  • Cluster provisioning       • Application deployments      │
│  • Cluster registration       • Continuous deployment        │
│  • BindingPolicies            • Deploys to WDS               │
│                                                               │
│         │                              │                      │
│         └──────────┬───────────────────┘                      │
│                    │                                          │
│                    ▼                                          │
│              KubeStellar                                      │
│         (Distributes to WECs)                                │
└──────────────────────────────────────────────────────────────┘
```

### Terraform Responsibilities

- Provision cloud infrastructure (VPCs, clusters)
- Register clusters with KubeStellar
- Create and manage BindingPolicies
- Set up initial configuration

### GitOps Responsibilities

- Deploy applications to WDS
- Manage application lifecycle
- Handle continuous deployment
- Maintain application configuration

---

## 11. Key Takeaways

| Concept | What It Means |
|---------|---------------|
| **Terraform Provider** | Plugin that lets Terraform manage a specific system |
| **KubeStellar** | Multi-cluster Kubernetes orchestration |
| **WDS** | Central hub where you define what to deploy |
| **ITS** | Registry of all clusters |
| **WEC** | Actual clusters where workloads run |
| **BindingPolicy** | Rules mapping workloads → clusters |
| **Labels** | Key-value pairs used for selection |
| **Downsync** | What resources to distribute |

### Benefits of This Integration

✅ **Version Control** - All infrastructure defined in code  
✅ **Repeatability** - Same config produces same results  
✅ **Automation** - CI/CD pipelines can apply changes  
✅ **Visibility** - `terraform plan` shows what will change  
✅ **Rollback** - Git history enables easy rollback  
✅ **Collaboration** - Teams can review infrastructure changes  

---

## Quick Reference Commands

```bash
# Initialize Terraform
terraform init

# Preview changes
terraform plan

# Apply changes
terraform apply

# Destroy resources
terraform destroy

# Import existing resource
terraform import kubestellar_cluster.my_cluster kubestellar/my-cluster-name

# Show current state
terraform show

# Refresh state from actual infrastructure
terraform refresh
```

---

## Additional Resources

- [Terraform Documentation](https://developer.hashicorp.com/terraform/docs)
- [KubeStellar Documentation](https://docs.kubestellar.io/)
- [Kubernetes Multi-Cluster Concepts](https://kubernetes.io/docs/concepts/cluster-administration/cluster-management/)
- [Terraform Provider Development](https://developer.hashicorp.com/terraform/plugin/framework)

---

*This documentation was generated to help understand the KubeStellar Terraform Provider project structure, concepts, and usage patterns.*
