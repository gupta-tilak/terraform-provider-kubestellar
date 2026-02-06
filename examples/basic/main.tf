# =============================================================================
# Simple KubeStellar BindingPolicy Example
# =============================================================================
# This example shows the basic usage of the KubeStellar provider
# to create a BindingPolicy that distributes workloads to production clusters.
# =============================================================================

terraform {
  required_version = ">= 1.0.0"

  required_providers {
    kubestellar = {
      source  = "kubestellar/kubestellar"
      version = "~> 0.1.0"
    }
  }
}

# Configure the KubeStellar provider
provider "kubestellar" {
  kubeconfig = "~/.kube/config"
}

# Create a BindingPolicy for production workloads
resource "kubestellar_binding_policy" "prod" {
  name      = "prod-policy"
  namespace = "default"

  # Select clusters with env=prod label
  cluster_selector {
    match_labels = {
      env = "prod"
    }
  }

  # Deploy apps/deployments with app=demo label
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

  # Also deploy associated services
  downsync {
    api_group  = ""
    resources  = ["services"]
    namespaces = ["default"]

    object_selectors {
      match_labels = {
        app = "demo"
      }
    }
  }
}

# Output the matched clusters
output "matched_clusters" {
  description = "Clusters that match this policy"
  value       = kubestellar_binding_policy.prod.matched_clusters
}
