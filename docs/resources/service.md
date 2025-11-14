# servicefabric_service (Resource)

Creates and manages a stateful or stateless Service Fabric service within an
existing application. The resource supports singleton, named, and
UniformInt64Range partitions plus the most frequently updated properties such as
instance/replica counts, placement constraints, DNS names, and default move
cost.

## Example Usage

```terraform
resource "servicefabric_service" "api" {
  name              = "${servicefabric_application.sample.name}/ApiService"
  application_name  = servicefabric_application.sample.name
  service_type_name = "Contoso.Sample.ApiServiceType"
  service_kind      = "Stateless"

  partition = {
    scheme = "Singleton"
  }

  stateless = {
    instance_count = 3
  }
}
```

## Argument Reference

The following arguments are supported:

- `name` (Required) – Fully-qualified service name (e.g. `fabric:/MyApp/MySvc`).
- `application_name` (Required) – Application that owns the service.
- `service_type_name` (Required) – Service type registered in the application
  manifest.
- `service_kind` (Required) – Either `Stateless` or `Stateful`.
- `placement_constraints` (Optional) – Node placement constraints.
- `default_move_cost` (Optional) – `Zero`, `Low`, `Medium`, `High`, or
  `VeryHigh`.
- `service_package_activation_mode` (Optional) – `SharedProcess` or
  `ExclusiveProcess`. Changing this value forces a new resource.
- `service_dns_name` (Optional) – DNS name assigned to the service.
- `force_remove` (Optional) – When true, destroy issues `ForceRemove=true`.
- `partition` (Required) – Object attribute describing partitioning:
  - `scheme` (Required) – `Singleton`, `Named`, or `UniformInt64Range`.
  - `count` (Optional) – Partition count (named or uniform schemes).
  - `names` (Optional) – List of partition names (named scheme).
  - `low_key` / `high_key` (Optional) – Key range (uniform scheme).
- `stateless` (Optional) – Required when `service_kind = "Stateless"`.
  Configure as `stateless = { ... }`. Supports:
  Supports:
  - `instance_count` (Optional) – Number of instances per partition (-1 deploys
    everywhere).
  - `min_instance_count` (Optional) – Minimum instances preserved during runs.
  - `min_instance_percentage` (Optional) – Minimum percentage preserved.
  - `instance_close_delay_seconds` (Optional) – Delay before closing on upgrade.
  - `instance_restart_wait_seconds` (Optional) – Restart delay for failed
    instances.
- `stateful` (Optional) – Required when `service_kind = "Stateful"`. Configure as `stateful = { ... }`. Supports:
  - `target_replica_set_size` (Required) – Replica count per partition.
  - `min_replica_set_size` (Required) – Minimum replicas needed for quorum.
  - `has_persisted_state` (Required) – Whether the service persists state.
  - `replica_restart_wait_seconds` (Optional) – Restart delay after failures.
  - `quorum_loss_wait_seconds` (Optional) – Wait before declaring quorum loss.
  - `standby_replica_keep_seconds` (Optional) – Retention period for standby
    replicas.
  - `service_placement_time_limit_seconds` (Optional) – Maximum wait for
    placement.

Changes to `partition` or `service_package_activation_mode` require creating a
new service. Instance/replica counts, placement constraints, default move cost,
and DNS name are updated in-place by calling the Service Fabric UpdateService
API.

## Attributes Reference

In addition to the arguments exported above, the following attributes are
exported:

- `id` – Service Fabric service name.
- `health_state` – Current health state as reported by the cluster.
- `service_status` – Provisioning status (`Active`, `Upgrading`, etc.).
