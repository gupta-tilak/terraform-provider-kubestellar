# =============================================================================
# End-to-End KubeStellar Workflow Example
# =============================================================================
# This example demonstrates the complete flow:
# 1. Terraform provisions clusters (using kind for demo)
# 2. Terraform registers clusters with KubeStellar
# 3. Terraform creates BindingPolicies
# 4. Sample workload is deployed to WDS
# 5. KubeStellar distributes to matched WECs
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
    null = {
      source  = "hashicorp/null"
      version = "~> 3.2"
    }
    local = {
      source  = "hashicorp/local"
      version = "~> 2.4"
    }
  }
}

# =============================================================================
# Variables
# =============================================================================

variable "create_kind_clusters" {
  description = "Whether to create kind clusters for demo"
  type        = bool
  default     = true
}

variable "wec_count" {
  description = "Number of WEC clusters to create"
  type        = number
  default     = 2
}

variable "kubeconfig_path" {
  type    = string
  default = "~/.kube/config"
}

# =============================================================================
# Local Values
# =============================================================================

locals {
  wec_clusters = {
    for i in range(var.wec_count) : "wec-${i + 1}" => {
      name = "wec-${i + 1}"
      env  = i == 0 ? "production" : "staging"
    }
  }
}

# =============================================================================
# Step 1: Provision Kind Clusters (Demo/Testing)
# =============================================================================

# Create kind clusters for WECs
resource "null_resource" "create_wec_clusters" {
  for_each = var.create_kind_clusters ? local.wec_clusters : {}

  provisioner "local-exec" {
    command = <<-EOT
      if ! kind get clusters | grep -q "^${each.value.name}$"; then
        kind create cluster --name ${each.value.name} --wait 60s
      fi
    EOT
  }

  provisioner "local-exec" {
    when    = destroy
    command = "kind delete cluster --name ${each.value.name} || true"
  }
}

# Get kubeconfig for each WEC
data "local_file" "wec_kubeconfigs" {
  for_each = var.create_kind_clusters ? local.wec_clusters : {}

  filename = pathexpand("~/.kube/kind-${each.value.name}.yaml")

  depends_on = [null_resource.create_wec_clusters]
}

# Export kubeconfig for each WEC
resource "null_resource" "export_kubeconfigs" {
  for_each = var.create_kind_clusters ? local.wec_clusters : {}

  provisioner "local-exec" {
    command = "kind get kubeconfig --name ${each.value.name} > ~/.kube/kind-${each.value.name}.yaml"
  }

  depends_on = [null_resource.create_wec_clusters]

  triggers = {
    cluster_name = each.value.name
  }
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

# =============================================================================
# Step 2: Register Clusters with KubeStellar
# =============================================================================

resource "kubestellar_its" "main" {
  name      = "demo-its"
  namespace = "kubestellar"

  labels = {
    demo = "true"
  }

  sync_interval = "30s"
}

resource "kubestellar_wds" "main" {
  name      = "demo-wds"
  namespace = "kubestellar"

  labels = {
    demo = "true"
  }
}

# Register WEC clusters
resource "kubestellar_cluster" "wecs" {
  for_each = local.wec_clusters

  name         = each.key
  namespace    = "kubestellar"
  cluster_name = each.key

  labels = {
    env      = each.value.env
    demo     = "true"
    workload = "general"
  }

  description = "Demo WEC cluster ${each.key}"
  environment = each.value.env

  # Use the exported kubeconfig
  kubeconfig = var.create_kind_clusters ? file(pathexpand("~/.kube/kind-${each.value.name}.yaml")) : ""

  depends_on = [
    kubestellar_its.main,
    null_resource.export_kubeconfigs,
  ]
}

# =============================================================================
# Step 3: Create BindingPolicies
# =============================================================================

resource "kubestellar_binding_policy" "demo_app" {
  name      = "demo-app-policy"
  namespace = "default"

  labels = {
    demo = "true"
  }

  cluster_selector {
    match_labels = {
      demo     = "true"
      workload = "general"
    }
  }

  downsync {
    api_group  = "apps"
    resources  = ["deployments"]
    namespaces = ["demo"]

    object_selectors {
      match_labels = {
        app = "demo"
      }
    }

    status_collection = "Full"
  }

  downsync {
    api_group  = ""
    resources  = ["services", "configmaps"]
    namespaces = ["demo"]

    object_selectors {
      match_labels = {
        app = "demo"
      }
    }
  }

  depends_on = [kubestellar_cluster.wecs]
}

resource "kubestellar_binding_policy" "prod_only" {
  name      = "prod-only-policy"
  namespace = "default"

  cluster_selector {
    match_labels = {
      env = "production"
    }
  }

  downsync {
    api_group  = "apps"
    resources  = ["deployments"]
    namespaces = ["production"]

    status_collection = "Full"
  }

  depends_on = [kubestellar_cluster.wecs]
}

# =============================================================================
# Step 4: Deploy Sample Workload to WDS
# =============================================================================

resource "kubernetes_namespace" "demo" {
  metadata {
    name = "demo"
    labels = {
      app = "demo"
    }
  }
}

resource "kubernetes_deployment" "demo_app" {
  metadata {
    name      = "demo-app"
    namespace = kubernetes_namespace.demo.metadata[0].name
    labels = {
      app = "demo"
    }
  }

  spec {
    replicas = 2

    selector {
      match_labels = {
        app = "demo"
      }
    }

    template {
      metadata {
        labels = {
          app = "demo"
        }
      }

      spec {
        container {
          name  = "nginx"
          image = "nginx:1.25"

          port {
            container_port = 80
          }

          resources {
            limits = {
              cpu    = "100m"
              memory = "128Mi"
            }
            requests = {
              cpu    = "50m"
              memory = "64Mi"
            }
          }
        }
      }
    }
  }

  depends_on = [kubestellar_binding_policy.demo_app]
}

resource "kubernetes_service" "demo_app" {
  metadata {
    name      = "demo-app"
    namespace = kubernetes_namespace.demo.metadata[0].name
    labels = {
      app = "demo"
    }
  }

  spec {
    selector = {
      app = "demo"
    }

    port {
      port        = 80
      target_port = 80
    }

    type = "ClusterIP"
  }
}

resource "kubernetes_config_map" "demo_config" {
  metadata {
    name      = "demo-config"
    namespace = kubernetes_namespace.demo.metadata[0].name
    labels = {
      app = "demo"
    }
  }

  data = {
    "config.json" = jsonencode({
      app_name    = "demo"
      environment = "multi-cluster"
      version     = "1.0.0"
    })
  }
}

# =============================================================================
# Step 5: Query Status
# =============================================================================

data "kubestellar_clusters" "all" {
  namespace = "kubestellar"

  depends_on = [kubestellar_cluster.wecs]
}

data "kubestellar_binding_policies" "all" {
  namespace = "default"

  depends_on = [
    kubestellar_binding_policy.demo_app,
    kubestellar_binding_policy.prod_only,
  ]
}

# =============================================================================
# Outputs
# =============================================================================

output "workflow_summary" {
  description = "End-to-end workflow summary"
  value = {
    step1_clusters_provisioned = var.create_kind_clusters ? keys(local.wec_clusters) : []
    step2_clusters_registered  = [for c in kubestellar_cluster.wecs : c.name]
    step3_policies_created = [
      kubestellar_binding_policy.demo_app.name,
      kubestellar_binding_policy.prod_only.name,
    ]
    step4_workload_deployed = kubernetes_deployment.demo_app.metadata[0].name
    step5_distribution_status = {
      demo_policy_matched_clusters = kubestellar_binding_policy.demo_app.matched_clusters
      prod_policy_matched_clusters = kubestellar_binding_policy.prod_only.matched_clusters
    }
  }
}

output "cluster_status" {
  description = "Status of all registered clusters"
  value = {
    for c in data.kubestellar_clusters.all.clusters : c.name => {
      ready              = c.ready
      kubernetes_version = c.kubernetes_version
      labels             = c.labels
    }
  }
}

output "next_steps" {
  description = "What happens next"
  value = <<-EOT
    ✅ Clusters provisioned and registered with KubeStellar
    ✅ BindingPolicies created for workload distribution
    ✅ Sample workload deployed to WDS
    
    KubeStellar will now:
    1. Match the demo-app workload to the BindingPolicy
    2. Identify target clusters (WECs) based on selectors
    3. Propagate the workload to matched clusters
    4. Collect status from WECs back to WDS
    
    Verify distribution:
    $ kubectl --context kind-wec-1 get deployments -n demo
    $ kubectl --context kind-wec-2 get deployments -n demo
  EOT
}
