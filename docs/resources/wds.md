# kubestellar_wds Resource

Manages a KubeStellar Workload Definition Space (WDS).

## Description

The Workload Definition Space is a Kubernetes-compatible API server that serves as the primary control plane for KubeStellar. Users apply their workloads (Deployments, Services, ConfigMaps, etc.) and BindingPolicies to the WDS, and KubeStellar distributes them to the appropriate Workload Execution Clusters (WECs).

## Example Usage

### Basic WDS

```hcl
resource "kubestellar_wds" "main" {
  name      = "main-wds"
  namespace = "kubestellar"

  labels = {
    environment = "production"
  }

  space_type = "vcluster"
}
```

### External WDS

```hcl
resource "kubestellar_wds" "external" {
  name      = "external-wds"
  namespace = "kubestellar"

  space_type   = "external"
  api_endpoint = "https://wds.example.com:6443"
  ca_data      = file("certs/wds-ca.pem")
  token_ref    = "kubestellar/wds-token"
}
```

## Argument Reference

### Required Arguments

* `name` - (Required, Forces new resource) The name of the WDS.

### Optional Arguments

* `namespace` - (Optional, Forces new resource) The namespace. Defaults to `"kubestellar"`.
* `labels` - (Optional) Map of labels to apply.
* `annotations` - (Optional) Map of annotations to apply.
* `space_type` - (Optional, Forces new resource) Type of the space implementation: `"vcluster"`, `"kind"`, `"external"`. Defaults to `"vcluster"`.
* `api_endpoint` - (Optional) API server endpoint URL. Required for `"external"` space type.
* `ca_data` - (Optional, Sensitive) PEM-encoded CA certificate data.
* `token_ref` - (Optional) Reference to a Secret containing authentication token.

## Attribute Reference

* `id` - The resource identifier.
* `status` - Current status (e.g., "Ready", "Provisioning", "Error").
* `ready` - Boolean indicating if WDS is ready.

## Import

```bash
terraform import kubestellar_wds.example kubestellar/my-wds
```
