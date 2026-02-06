# =============================================================================
# KubeStellar + GitOps Combined Example
# =============================================================================
# This example demonstrates how to use KubeStellar Terraform provider
# alongside GitOps tools (Argo CD or Flux) for a complete multi-cluster
# deployment workflow.
#
# Workflow:
# 1. Terraform provisions clusters and registers them with KubeStellar
# 2. Terraform creates BindingPolicies for workload distribution
# 3. GitOps tools deploy workloads to WDS
# 4. KubeStellar distributes workloads to matched WECs
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
    helm = {
      source  = "hashicorp/helm"
      version = "~> 2.11"
    }
  }
}

# =============================================================================
# Variables
# =============================================================================

variable "kubeconfig_path" {
  type    = string
  default = "~/.kube/config"
}

variable "argocd_namespace" {
  type    = string
  default = "argocd"
}

variable "flux_namespace" {
  type    = string
  default = "flux-system"
}

variable "gitops_tool" {
  description = "GitOps tool to use: 'argocd' or 'flux'"
  type        = string
  default     = "argocd"
}

variable "git_repo_url" {
  description = "Git repository URL containing workload definitions"
  type        = string
  default     = "https://github.com/example/kubestellar-workloads.git"
}

variable "git_repo_branch" {
  type    = string
  default = "main"
}

# =============================================================================
# Providers
# =============================================================================

provider "kubestellar" {
  kubeconfig = var.kubeconfig_path
}

provider "kubernetes" {
  config_path = var.kubeconfig_path
}

provider "helm" {
  kubernetes {
    config_path = var.kubeconfig_path
  }
}

# =============================================================================
# KubeStellar Infrastructure
# =============================================================================

# WDS - Where GitOps tools deploy workloads
resource "kubestellar_wds" "main" {
  name      = "main-wds"
  namespace = "kubestellar"

  labels = {
    environment = "production"
    managed-by  = "terraform"
  }

  space_type = "vcluster"
}

# ITS - Cluster inventory
resource "kubestellar_its" "main" {
  name      = "main-its"
  namespace = "kubestellar"

  labels = {
    environment = "production"
  }

  sync_interval = "30s"
}

# =============================================================================
# Register WECs (Workload Execution Clusters)
# =============================================================================

locals {
  clusters = {
    "prod-cluster-1" = {
      env      = "production"
      region   = "us-east-1"
      tier     = "frontend"
    }
    "prod-cluster-2" = {
      env      = "production"
      region   = "us-west-2"
      tier     = "frontend"
    }
    "staging-cluster" = {
      env      = "staging"
      region   = "us-central-1"
      tier     = "all"
    }
  }
}

resource "kubestellar_cluster" "wecs" {
  for_each = local.clusters

  name         = each.key
  namespace    = "kubestellar"
  cluster_name = each.key

  labels = each.value

  description = "Cluster ${each.key} for ${each.value.env}"
  environment = each.value.env

  # In real usage, provide actual kubeconfig
  api_endpoint = "https://${each.key}.example.com:6443"
  token_ref    = "kubestellar/${each.key}-token"

  depends_on = [kubestellar_its.main]
}

# =============================================================================
# BindingPolicies - Define workload distribution rules
# =============================================================================

# Policy for production frontend apps
resource "kubestellar_binding_policy" "frontend_prod" {
  name      = "frontend-production"
  namespace = "default"

  cluster_selector {
    match_labels = {
      env  = "production"
      tier = "frontend"
    }
  }

  # Apps namespace - managed by GitOps
  downsync {
    api_group  = "apps"
    resources  = ["deployments", "replicasets"]
    namespaces = ["frontend-apps"]

    status_collection = "Full"
  }

  downsync {
    api_group  = ""
    resources  = ["services", "configmaps", "secrets"]
    namespaces = ["frontend-apps"]
  }

  depends_on = [kubestellar_cluster.wecs]
}

# Policy for staging - deploy everything for testing
resource "kubestellar_binding_policy" "staging" {
  name      = "staging-all"
  namespace = "default"

  cluster_selector {
    match_labels = {
      env = "staging"
    }
  }

  downsync {
    api_group  = "apps"
    resources  = ["deployments", "statefulsets", "daemonsets", "replicasets"]
    namespaces = ["frontend-apps", "backend-apps", "staging"]

    status_collection = "Full"
  }

  downsync {
    api_group = ""
    resources = ["services", "configmaps", "secrets", "persistentvolumeclaims"]
    namespaces = ["frontend-apps", "backend-apps", "staging"]
  }

  depends_on = [kubestellar_cluster.wecs]
}

# =============================================================================
# Argo CD Integration (if gitops_tool == "argocd")
# =============================================================================

# Install Argo CD (if not already installed)
resource "helm_release" "argocd" {
  count = var.gitops_tool == "argocd" ? 1 : 0

  name             = "argocd"
  repository       = "https://argoproj.github.io/argo-helm"
  chart            = "argo-cd"
  version          = "5.51.0"
  namespace        = var.argocd_namespace
  create_namespace = true

  set {
    name  = "server.service.type"
    value = "ClusterIP"
  }
}

# Argo CD Application - deploys to WDS
resource "kubernetes_manifest" "argocd_application" {
  count = var.gitops_tool == "argocd" ? 1 : 0

  manifest = {
    apiVersion = "argoproj.io/v1alpha1"
    kind       = "Application"
    metadata = {
      name      = "kubestellar-workloads"
      namespace = var.argocd_namespace
    }
    spec = {
      project = "default"
      source = {
        repoURL        = var.git_repo_url
        targetRevision = var.git_repo_branch
        path           = "apps"
      }
      destination = {
        # Deploy to WDS - KubeStellar will distribute
        server    = "https://kubernetes.default.svc"
        namespace = "frontend-apps"
      }
      syncPolicy = {
        automated = {
          prune    = true
          selfHeal = true
        }
        syncOptions = ["CreateNamespace=true"]
      }
    }
  }

  depends_on = [helm_release.argocd, kubestellar_wds.main]
}

# Argo CD Project for KubeStellar workloads
resource "kubernetes_manifest" "argocd_project" {
  count = var.gitops_tool == "argocd" ? 1 : 0

  manifest = {
    apiVersion = "argoproj.io/v1alpha1"
    kind       = "AppProject"
    metadata = {
      name      = "kubestellar-apps"
      namespace = var.argocd_namespace
    }
    spec = {
      description = "KubeStellar managed applications"
      sourceRepos = [var.git_repo_url]
      destinations = [
        {
          namespace = "*"
          server    = "*"
        }
      ]
      clusterResourceWhitelist = [
        {
          group = "*"
          kind  = "*"
        }
      ]
    }
  }

  depends_on = [helm_release.argocd]
}

# =============================================================================
# Flux Integration (if gitops_tool == "flux")
# =============================================================================

# Flux GitRepository - source for workloads
resource "kubernetes_manifest" "flux_gitrepo" {
  count = var.gitops_tool == "flux" ? 1 : 0

  manifest = {
    apiVersion = "source.toolkit.fluxcd.io/v1"
    kind       = "GitRepository"
    metadata = {
      name      = "kubestellar-workloads"
      namespace = var.flux_namespace
    }
    spec = {
      interval = "1m"
      url      = var.git_repo_url
      ref = {
        branch = var.git_repo_branch
      }
    }
  }
}

# Flux Kustomization - deploys to WDS
resource "kubernetes_manifest" "flux_kustomization" {
  count = var.gitops_tool == "flux" ? 1 : 0

  manifest = {
    apiVersion = "kustomize.toolkit.fluxcd.io/v1"
    kind       = "Kustomization"
    metadata = {
      name      = "kubestellar-apps"
      namespace = var.flux_namespace
    }
    spec = {
      interval = "5m"
      path     = "./apps"
      prune    = true
      sourceRef = {
        kind = "GitRepository"
        name = "kubestellar-workloads"
      }
      targetNamespace = "frontend-apps"
    }
  }

  depends_on = [kubernetes_manifest.flux_gitrepo, kubestellar_wds.main]
}

# =============================================================================
# Outputs
# =============================================================================

output "wds_endpoint" {
  description = "WDS API endpoint for GitOps tools"
  value       = kubestellar_wds.main.api_endpoint
}

output "registered_clusters" {
  description = "All registered WECs"
  value = {
    for name, cluster in kubestellar_cluster.wecs : name => {
      ready  = cluster.ready
      status = cluster.status
    }
  }
}

output "binding_policies" {
  description = "Active binding policies"
  value = {
    frontend = {
      matched_clusters = kubestellar_binding_policy.frontend_prod.matched_clusters
    }
    staging = {
      matched_clusters = kubestellar_binding_policy.staging.matched_clusters
    }
  }
}

output "gitops_integration" {
  description = "GitOps tool configuration"
  value = {
    tool       = var.gitops_tool
    repo_url   = var.git_repo_url
    branch     = var.git_repo_branch
    namespace  = var.gitops_tool == "argocd" ? var.argocd_namespace : var.flux_namespace
  }
}
