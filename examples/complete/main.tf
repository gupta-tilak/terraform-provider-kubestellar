# =============================================================================
# Complete KubeStellar Multi-Cluster Infrastructure Example
# =============================================================================
# This example demonstrates:
# 1. Provisioning the KubeStellar control plane (WDS + ITS)
# 2. Registering multiple clusters with different labels
# 3. Creating BindingPolicies for workload distribution
# 4. Using data sources to query cluster status
# =============================================================================

terraform {
  required_version = ">= 1.0.0"

  required_providers {
    kubestellar = {
      source  = "kubestellar/kubestellar"
      version = "~> 0.1.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.23"
    }
  }
}

# =============================================================================
# Provider Configuration
# =============================================================================

provider "kubestellar" {
  kubeconfig   = var.kubeconfig_path
  context      = var.kubeconfig_context
  validate_crds = true

  # Optional: Separate contexts for WDS and ITS if using different clusters
  # wds_context = "wds-cluster"
  # its_context = "its-cluster"
}

provider "kubernetes" {
  config_path    = var.kubeconfig_path
  config_context = var.kubeconfig_context
}

# =============================================================================
# Variables
# =============================================================================

variable "kubeconfig_path" {
  description = "Path to the kubeconfig file"
  type        = string
  default     = "~/.kube/config"
}

variable "kubeconfig_context" {
  description = "Kubernetes context to use"
  type        = string
  default     = ""
}

variable "environment" {
  description = "Environment name"
  type        = string
  default     = "demo"
}

variable "clusters" {
  description = "Map of clusters to register with KubeStellar"
  type = map(object({
    kubeconfig     = string
    location       = string
    cloud_provider = string
    environment    = string
    labels         = map(string)
  }))
  default = {}
}

# =============================================================================
# KubeStellar Control Plane
# =============================================================================

# Create the Workload Definition Space (WDS)
resource "kubestellar_wds" "main" {
  name      = "main-wds"
  namespace = "kubestellar"

  labels = {
    "app.kubernetes.io/name"      = "kubestellar"
    "app.kubernetes.io/component" = "wds"
    environment                   = var.environment
  }

  space_type = "vcluster"
}

# Create the Inventory and Transport Space (ITS)
resource "kubestellar_its" "main" {
  name      = "main-its"
  namespace = "kubestellar"

  labels = {
    "app.kubernetes.io/name"      = "kubestellar"
    "app.kubernetes.io/component" = "its"
    environment                   = var.environment
  }

  space_type    = "vcluster"
  sync_interval = "30s"
}

# =============================================================================
# Cluster Registration
# =============================================================================

# Register clusters from the variable map
resource "kubestellar_cluster" "clusters" {
  for_each = var.clusters

  name        = each.key
  namespace   = "kubestellar"
  cluster_name = each.key

  labels = merge(each.value.labels, {
    "kubestellar.io/managed-by" = "terraform"
  })

  description    = "Cluster ${each.key} managed by Terraform"
  location       = each.value.location
  cloud_provider = each.value.cloud_provider
  environment    = each.value.environment

  kubeconfig = each.value.kubeconfig

  depends_on = [kubestellar_its.main]
}

# Example: Register specific named clusters
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

  description    = "Production cluster in AWS US East 1"
  location       = "us-east-1"
  cloud_provider = "aws"
  environment    = "production"

  kubeconfig = file("${path.module}/kubeconfigs/prod-us-east-1.yaml")

  depends_on = [kubestellar_its.main]
}

resource "kubestellar_cluster" "prod_us_west" {
  name         = "prod-us-west-2"
  namespace    = "kubestellar"
  cluster_name = "prod-us-west-2"

  labels = {
    env      = "production"
    region   = "us-west-2"
    tier     = "frontend"
    provider = "aws"
  }

  description    = "Production cluster in AWS US West 2"
  location       = "us-west-2"
  cloud_provider = "aws"
  environment    = "production"

  kubeconfig = file("${path.module}/kubeconfigs/prod-us-west-2.yaml")

  depends_on = [kubestellar_its.main]
}

resource "kubestellar_cluster" "staging" {
  name         = "staging-central"
  namespace    = "kubestellar"
  cluster_name = "staging-central"

  labels = {
    env      = "staging"
    region   = "us-central-1"
    tier     = "all"
    provider = "gcp"
  }

  description    = "Staging cluster in GCP US Central"
  location       = "us-central-1"
  cloud_provider = "gcp"
  environment    = "staging"

  kubeconfig = file("${path.module}/kubeconfigs/staging-central.yaml")

  depends_on = [kubestellar_its.main]
}

# =============================================================================
# Binding Policies
# =============================================================================

# Policy: Deploy frontend apps to production clusters
resource "kubestellar_binding_policy" "frontend_prod" {
  name      = "frontend-production"
  namespace = "default"

  labels = {
    "kubestellar.io/managed-by" = "terraform"
    "policy-type"               = "frontend"
  }

  cluster_selector {
    match_labels = {
      env  = "production"
      tier = "frontend"
    }
  }

  # Deploy Deployments with 'tier=frontend' label
  downsync {
    api_group   = "apps"
    resources   = ["deployments", "replicasets"]
    namespaces  = ["frontend", "default"]
    
    object_selectors {
      match_labels = {
        tier = "frontend"
      }
    }

    status_collection = "Full"
  }

  # Deploy associated Services
  downsync {
    api_group  = ""
    resources  = ["services"]
    namespaces = ["frontend", "default"]

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
    namespaces = ["frontend", "default"]

    object_selectors {
      match_labels = {
        tier = "frontend"
      }
    }
  }

  depends_on = [
    kubestellar_cluster.prod_us_east,
    kubestellar_cluster.prod_us_west,
  ]
}

# Policy: Deploy to all US regions
resource "kubestellar_binding_policy" "us_regions" {
  name      = "us-region-apps"
  namespace = "default"

  cluster_selector {
    match_expressions {
      key      = "region"
      operator = "In"
      values   = ["us-east-1", "us-west-2", "us-central-1"]
    }
  }

  downsync {
    api_group  = "apps"
    resources  = ["deployments"]
    namespaces = ["regional-apps"]

    object_selectors {
      match_labels = {
        distribution = "regional"
      }
    }

    status_collection = "Full"
  }
}

# Policy: Deploy to staging for testing
resource "kubestellar_binding_policy" "staging_all" {
  name      = "staging-all-apps"
  namespace = "default"

  cluster_selector {
    match_labels = {
      env = "staging"
    }
  }

  # Deploy all apps from the staging namespace
  downsync {
    api_group  = "apps"
    resources  = ["deployments", "statefulsets", "daemonsets"]
    namespaces = ["staging"]

    status_collection = "Full"
  }

  downsync {
    api_group  = ""
    resources  = ["services", "configmaps", "secrets", "persistentvolumeclaims"]
    namespaces = ["staging"]
  }

  depends_on = [kubestellar_cluster.staging]
}

# Policy: Singleton service (only one instance across all clusters)
resource "kubestellar_binding_policy" "singleton_service" {
  name      = "singleton-controller"
  namespace = "default"

  want_singleton_reported_state = true

  cluster_selector {
    match_labels = {
      env = "production"
    }
  }

  downsync {
    api_group   = "apps"
    resources   = ["deployments"]
    namespaces  = ["controllers"]
    object_names = ["singleton-controller"]

    status_collection = "Full"
  }
}

# =============================================================================
# Data Sources - Query Cluster Information
# =============================================================================

# Get all registered clusters
data "kubestellar_clusters" "all" {
  namespace = "kubestellar"

  depends_on = [
    kubestellar_cluster.prod_us_east,
    kubestellar_cluster.prod_us_west,
    kubestellar_cluster.staging,
  ]
}

# Get production clusters only
data "kubestellar_clusters" "production" {
  namespace = "kubestellar"
  labels = {
    env = "production"
  }

  depends_on = [
    kubestellar_cluster.prod_us_east,
    kubestellar_cluster.prod_us_west,
  ]
}

# Get all binding policies
data "kubestellar_binding_policies" "all" {
  namespace = "default"

  depends_on = [
    kubestellar_binding_policy.frontend_prod,
    kubestellar_binding_policy.us_regions,
    kubestellar_binding_policy.staging_all,
  ]
}

# =============================================================================
# Outputs
# =============================================================================

output "wds_status" {
  description = "Status of the Workload Definition Space"
  value = {
    name   = kubestellar_wds.main.name
    ready  = kubestellar_wds.main.ready
    status = kubestellar_wds.main.status
  }
}

output "its_status" {
  description = "Status of the Inventory and Transport Space"
  value = {
    name          = kubestellar_its.main.name
    ready         = kubestellar_its.main.ready
    status        = kubestellar_its.main.status
    cluster_count = kubestellar_its.main.cluster_count
  }
}

output "all_cluster_names" {
  description = "Names of all registered clusters"
  value       = [for c in data.kubestellar_clusters.all.clusters : c.name]
}

output "production_clusters" {
  description = "Production cluster details"
  value = {
    for c in data.kubestellar_clusters.production.clusters : c.name => {
      ready              = c.ready
      kubernetes_version = c.kubernetes_version
      labels             = c.labels
    }
  }
}

output "binding_policies" {
  description = "Active binding policies"
  value = {
    for p in data.kubestellar_binding_policies.all.policies : p.name => {
      matched_clusters  = p.matched_clusters
      matched_workloads = p.matched_workloads
    }
  }
}

output "frontend_policy_matched_clusters" {
  description = "Clusters matched by the frontend production policy"
  value       = kubestellar_binding_policy.frontend_prod.matched_clusters
}
