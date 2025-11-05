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
}
```

## Argument Reference

- `name` (Required) – Fully qualified application name including the `fabric:`
  prefix.
- `type_name` (Required) – Application type name registered in the cluster.
- `type_version` (Required) – Application type version to deploy.
- `parameters` (Optional) – Map of parameter overrides defined in the
  application manifest.

## Attribute Reference

In addition to the arguments above, the following attributes are exported:

- `id` – Application name.
- `status` – Current status (for example `Ready`, `Upgrading`).
- `health_state` – Reported health state (`Ok`, `Warning`, `Error`, etc.).

## Import

Applications can be imported by name:

```shell
terraform import servicefabric_application.sample fabric:/Contoso.Sample
```
