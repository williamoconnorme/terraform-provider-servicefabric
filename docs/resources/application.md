# servicefabric_application

Deploys or updates a Service Fabric application instance using the specified
application type. Updates are applied by reissuing the create call with the new
parameters.

## Example Usage

```terraform
resource "servicefabric_application" "sample" {
  name         = "fabric:/Contoso.Sample"
  type_name    = servicefabric_application_type.sample.name
  type_version = servicefabric_application_type.sample.version

  parameters = {
    Environment   = "dev"
    InstanceCount = "3"
  }

  application_capacity {
    minimum_nodes = 2
    maximum_nodes = 5

    application_metrics {
      name                    = "CustomMetric"
      maximum_capacity        = 50
      reservation_capacity    = 10
      total_application_capacity = 250
    }
  }

  managed_application_identity {
    token_service_endpoint = "https://cluster.example.com:19080/TokenService"

    identities = [
      "MyUserAssignedIdentity",
      "00000000-0000-0000-0000-000000000000",
    ]
  }

  upgrade_policy {
    force_restart = false
    upgrade_mode  = "Monitored"
  }
}
```

## Argument Reference

- `name` (Required) – Fully qualified application name including the `fabric:`
  prefix.
- `type_name` (Required) – Application type name registered in the cluster.
- `type_version` (Required) – Application type version to deploy.
- `parameters` (Optional) – Map of parameter overrides defined in the
  application manifest.
- `application_capacity` (Optional) – Nested block defining capacity
  reservations and limits for the application:
  - `minimum_nodes` (Optional) – Minimum number of nodes where capacity is
    reserved.
  - `maximum_nodes` (Optional) – Maximum number of nodes where capacity can be
    reserved (set to `0` for no limit).
  - `application_metrics` (Optional) – List of custom metric capacity settings.
    Each entry supports:
    - `name` (Required) – Metric name.
    - `maximum_capacity` (Optional) – Maximum per-node capacity for the metric
      (set to `0` for no limit).
    - `reservation_capacity` (Optional) – Reserved per-node capacity for the
      metric.
    - `total_application_capacity` (Optional) – Total cluster-wide capacity for
      the metric (set to `0` for no limit).
- `managed_application_identity` (Optional) – Nested block configuring managed
  identities associated with the application:
  - `token_service_endpoint` (Optional) – Token service endpoint used for
    identity federation.
  - `identities` (Optional) – List of managed identity resource names or
    principal IDs (GUIDs) to associate with the application.
- `upgrade_policy` (Optional) – Controls how upgrades are applied when
  `type_version` or `parameters` change:
  - `force_restart` (Optional) – When `true`, Service Fabric forcefully restarts
    code packages instead of waiting for graceful shutdown.
  - `upgrade_mode` (Optional) – Upgrade mode. Allowed values:
    `UnmonitoredAuto`, `UnmonitoredManual`, or `Monitored`.
  - `monitoring_policy` (Optional) – Nested block with advanced timeouts:
    - `failure_action` – `Rollback` or `Manual`.
    - `health_check_wait_duration`,
      `health_check_stable_duration`,
      `health_check_retry_timeout`,
      `upgrade_timeout`,
      `upgrade_domain_timeout` – ISO8601 durations (e.g. `PT5M`).
  - `application_health_policy` (Optional) – Nested block with:
    - `consider_warning_as_error` (Optional) – Treat warnings as errors during
      upgrades.
    - `max_percent_unhealthy_deployed_applications` (Optional) – Maximum
      percentage of unhealthy deployed applications allowed.

## Attribute Reference

In addition to the arguments above, the following attributes are exported:

- `id` – Application name.
- `status` – Current status (for example `Ready`, `Upgrading`).
- `health_state` – Reported health state (`Ok`, `Warning`, `Error`, etc.).

## Import

Applications can be imported by composite identifier or by name (the latter requires `type_name` in configuration):

```shell
terraform import servicefabric_application.sample Contoso.SampleAppType|fabric:/Contoso.Sample
```
