# servicefabric_application (Data Source)

Returns information about an application deployed to the Service Fabric
cluster.

## Example Usage

```terraform
data "servicefabric_application" "sample" {
  name = "fabric:/Contoso.Sample"
}

output "application_status" {
  value = data.servicefabric_application.sample.status
}
```

## Argument Reference

- `name` (Required) – Fully qualified Service Fabric application name.

## Attribute Reference

- `id` – Application name.
- `type_name` – Application type name.
- `type_version` – Application type version.
- `status` – Current status.
- `health_state` – Reported health state.
- `parameters` – Map of parameter overrides applied to the application.
