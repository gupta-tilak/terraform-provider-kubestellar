# kubestellar_binding_policies Data Source

Lists BindingPolicies in KubeStellar.

## Description

This data source retrieves information about BindingPolicies defined in a namespace.

## Example Usage

```hcl
data "kubestellar_binding_policies" "all" {
  namespace = "default"
}

output "policies" {
  value = {
    for p in data.kubestellar_binding_policies.all.policies : p.name => {
      matched_clusters  = p.matched_clusters
      matched_workloads = p.matched_workloads
    }
  }
}
```

## Argument Reference

* `namespace` - (Optional) Namespace to list policies from.

## Attribute Reference

* `id` - Data source identifier.
* `policies` - List of policy objects with:
  * `name` - Policy name.
  * `namespace` - Policy namespace.
  * `matched_clusters` - Count of matched clusters.
  * `matched_workloads` - Count of matched workloads.
