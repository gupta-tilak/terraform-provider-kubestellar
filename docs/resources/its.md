# kubestellar_its Resource

Manages a KubeStellar Inventory and Transport Space (ITS).

## Description

The Inventory and Transport Space is the central registry for all Workload Execution Clusters (WECs) managed by KubeStellar. It maintains cluster inventory, stores metadata and labels, handles workload transport, and aggregates status from WECs.

## Example Usage

### Basic ITS

```hcl
resource "kubestellar_its" "main" {
  name      = "main-its"
  namespace = "kubestellar"

  labels = {
    environment = "production"
  }

  space_type    = "vcluster"
  sync_interval = "30s"
}
```

### External ITS

```hcl
resource "kubestellar_its" "external" {
  name      = "external-its"
  namespace = "kubestellar"

  space_type    = "external"
  api_endpoint  = "https://its.example.com:6443"
  sync_interval = "1m"
}
```

## Argument Reference

### Required Arguments

* `name` - (Required, Forces new resource) The name of the ITS.

### Optional Arguments

* `namespace` - (Optional, Forces new resource) The namespace. Defaults to `"kubestellar"`.
* `labels` - (Optional) Map of labels to apply.
* `annotations` - (Optional) Map of annotations to apply.
* `space_type` - (Optional, Forces new resource) Type: `"vcluster"`, `"kind"`, `"external"`. Defaults to `"vcluster"`.
* `api_endpoint` - (Optional) API endpoint for external type.
* `ca_data` - (Optional, Sensitive) PEM-encoded CA certificate.
* `token_ref` - (Optional) Reference to authentication Secret.
* `sync_interval` - (Optional) Cluster sync interval (e.g., `"30s"`, `"1m"`). Defaults to `"30s"`.

## Attribute Reference

* `id` - The resource identifier.
* `status` - Current status.
* `ready` - Boolean indicating if ITS is ready.
* `cluster_count` - Number of registered clusters.

## Import

```bash
terraform import kubestellar_its.example kubestellar/my-its
```
